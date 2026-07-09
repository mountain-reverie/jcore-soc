/* eth_report.c -- W5500 UDP driver over the spi2 master (iCESugar/J1).
   Provides the STRONG report_result() that overrides core_portme.c's weak
   no-op stub, plus eth_init().

   SPI + W5500 common-block write layer is copied from the
   hardware-confirmed banner.c driver (targets/boards/icesugar/rom/banner.c,
   w5500_init_ping()/w5500_write()/spi_byte()); this file adds BSB-aware
   access for socket-0 registers and the socket-0 TX buffer, plus reads.

   W5500 control byte = [BSB(7:3)][RWB(2)][OM(1:0)] (VDM: OM=00). BSB
   values (see W5500 datasheet, cross-checked against the write-only
   control decode in components/emac/w5500_model.vhd which confirms
   RWB=bit2, common block == BSB 0):
     BSB_COMMON   = 0x00  (common register block)
     BSB_SOCK0_REG= 0x01  (socket 0 register block)
     BSB_SOCK0_TX = 0x02  (socket 0 TX buffer)
   -> write control bytes: common=0x04, sock0-reg=0x0C, sock0-tx=0x14
      read  control bytes: common=0x00, sock0-reg=0x08, sock0-tx=0x10
   (0x0C/0x14 match the values called out in the task brief.)

   HOST_TEST: the raw MMIO spi2 pointers are replaced by a small in-memory
   W5500 register/TX-buffer model (see the #ifdef HOST_TEST block below) so
   the driver logic can be exercised on the host without hardware. */

#include <stddef.h>
#include "eth_report.h"

#ifndef HOST_TEST

/* ---- real hardware: spi2 master @ 0xABCD1000, identical semantics to
   banner.c (confirmed against components/misc/spi2.vhd). ---- */
#define ETH_BASE   0xABCD1000u
#define ETH_CTRL   (*(volatile unsigned int *)(ETH_BASE + 0x0u))
#define ETH_DATA   (*(volatile unsigned int *)(ETH_BASE + 0x4u))
#define ETH_CTRL_CS0    0x1u
#define ETH_CTRL_START  0x2u
#define ETH_CTRL_BUSY   0x2u

#define CYCCNT_ADDR 0xabcd0200u
#define CYCCNT (*(volatile unsigned int *)CYCCNT_ADDR)

static void spi_assert(void)   { ETH_CTRL = 0u; }
static void spi_deassert(void) { ETH_CTRL = ETH_CTRL_CS0; }

static unsigned char spi_byte(unsigned char txval)
{
	ETH_DATA = txval;
	ETH_CTRL = ETH_CTRL_START;
	while (ETH_CTRL & ETH_CTRL_BUSY)
		;
	return (unsigned char)ETH_DATA;
}

#else /* HOST_TEST: fake W5500 model driven by the same SPI byte protocol */

unsigned char hm_common[0x40];
unsigned char hm_sock0reg[0x40];
unsigned char hm_sock0tx[2048];

static int hm_phase;          /* 0=addr_hi 1=addr_lo 2=control 3.. =data */
static unsigned int hm_addr;
static unsigned int hm_bsb;
static unsigned int hm_rwb;

static void spi_assert(void)   { hm_phase = 0; }
static void spi_deassert(void) { }

static unsigned char *hm_block(void)
{
	switch (hm_bsb) {
	case 0: return hm_common;
	case 1: return hm_sock0reg;
	case 2: return hm_sock0tx;
	default: return hm_common;
	}
}

static unsigned char spi_byte(unsigned char txval)
{
	if (hm_phase == 0) {
		hm_addr = (unsigned int)txval << 8;
		hm_phase = 1;
		return 0;
	}
	if (hm_phase == 1) {
		hm_addr |= txval;
		hm_phase = 2;
		return 0;
	}
	if (hm_phase == 2) {
		hm_bsb = (txval >> 3) & 0x1Fu;
		hm_rwb = (txval >> 2) & 0x1u;
		hm_phase = 3;
		return 0;
	}

	{
		unsigned char *blk = hm_block();
		unsigned int mask = (hm_bsb == 2) ? (sizeof(hm_sock0tx) - 1u)
		                                   : 0x3Fu;
		unsigned int a = hm_addr & mask;
		unsigned char ret = 0;

		if (hm_rwb) {
			if (hm_bsb == 1 && hm_addr == 0x0002u)
				blk[a] &= (unsigned char)~txval; /* Sn_IR: write-1-to-clear */
			else
				blk[a] = txval;
			/* model the chip's autonomous socket-0 reactions so
			   the driver's polling loops make progress. */
			if (hm_bsb == 1 && hm_addr == 0x0001u) {
				if (txval == 0x01u)         /* Sn_CR = OPEN */
					hm_sock0reg[0x03] = 0x22u; /* Sn_SR = SOCK_UDP */
				else if (txval == 0x20u)    /* Sn_CR = SEND */
					hm_sock0reg[0x02] |= 0x10u; /* Sn_IR.SENDOK */
			}
		} else {
			ret = blk[a];
		}
		hm_addr++;
		return ret;
	}
}

#endif /* HOST_TEST */

#define BSB_COMMON    0x00u
#define BSB_SOCK0_REG 0x01u
#define BSB_SOCK0_TX  0x02u

static void w5500_wr(unsigned int addr, unsigned int bsb,
                      const unsigned char *buf, int n)
{
	int i;
	spi_assert();
	spi_byte((unsigned char)(addr >> 8));
	spi_byte((unsigned char)(addr & 0xffu));
	spi_byte((unsigned char)((bsb << 3) | 0x04u)); /* RWB=1, OM=VDM */
	for (i = 0; i < n; i++)
		spi_byte(buf[i]);
	spi_deassert();
}

static void w5500_wr8(unsigned int addr, unsigned int bsb, unsigned char v)
{
	w5500_wr(addr, bsb, &v, 1);
}

static void w5500_wr16(unsigned int addr, unsigned int bsb, unsigned int v)
{
	unsigned char b[2];
	b[0] = (unsigned char)(v >> 8);
	b[1] = (unsigned char)(v & 0xffu);
	w5500_wr(addr, bsb, b, 2);
}

static void w5500_rd(unsigned int addr, unsigned int bsb,
                      unsigned char *buf, int n)
{
	int i;
	spi_assert();
	spi_byte((unsigned char)(addr >> 8));
	spi_byte((unsigned char)(addr & 0xffu));
	spi_byte((unsigned char)(bsb << 3)); /* RWB=0, OM=VDM */
	for (i = 0; i < n; i++)
		buf[i] = spi_byte(0);
	spi_deassert();
}

static unsigned char w5500_rd8(unsigned int addr, unsigned int bsb)
{
	unsigned char v;
	w5500_rd(addr, bsb, &v, 1);
	return v;
}

static unsigned int w5500_rd16(unsigned int addr, unsigned int bsb)
{
	unsigned char b[2];
	w5500_rd(addr, bsb, b, 2);
	return ((unsigned int)b[0] << 8) | b[1];
}

#define SRC_UDP_PORT 47001u  /* fixed source port for the board's socket 0 */

static int eth_inited;

void eth_init(void)
{
	static const unsigned char shar[6] = CMK_BOARD_MAC;
	static const unsigned char sipr[4] = CMK_BOARD_IP;
	static const unsigned char subr[4] = CMK_SUBNET;
	static const unsigned char gar[4]  = CMK_GATEWAY;
	volatile unsigned int d;

	if (eth_inited)
		return;

	w5500_wr8(0x0000u, BSB_COMMON, 0x80u);  /* MR.RST */
	for (d = 0; d < 2000u; d++)
		;                                /* let RST self-clear */

	w5500_wr(0x0009u, BSB_COMMON, shar, 6); /* SHAR */
	w5500_wr(0x000Fu, BSB_COMMON, sipr, 4); /* SIPR */
	w5500_wr(0x0005u, BSB_COMMON, subr, 4); /* SUBR */
	w5500_wr(0x0001u, BSB_COMMON, gar, 4);  /* GAR */

	w5500_wr8(0x0000u, BSB_SOCK0_REG, 0x02u);       /* Sn_MR = UDP */
	w5500_wr16(0x0004u, BSB_SOCK0_REG, SRC_UDP_PORT); /* Sn_PORT */
	w5500_wr8(0x0001u, BSB_SOCK0_REG, 0x01u);       /* Sn_CR = OPEN */
	while (w5500_rd8(0x0003u, BSB_SOCK0_REG) != 0x22u) /* Sn_SR==SOCK_UDP */
		;

	eth_inited = 1;
}

static void put_le16(unsigned char *b, unsigned int v)
{
	b[0] = (unsigned char)(v & 0xffu);
	b[1] = (unsigned char)((v >> 8) & 0xffu);
}

static void put_le32(unsigned char *b, unsigned int v)
{
	b[0] = (unsigned char)(v & 0xffu);
	b[1] = (unsigned char)((v >> 8) & 0xffu);
	b[2] = (unsigned char)((v >> 16) & 0xffu);
	b[3] = (unsigned char)((v >> 24) & 0xffu);
}

static void send_once(struct coremark_result *r)
{
	static const unsigned char dipr[4] = CMK_COLLECTOR_IP;
	unsigned int wr;
	unsigned char buf[24];

	w5500_wr(0x000Cu, BSB_SOCK0_REG, dipr, 4);            /* Sn_DIPR */
	w5500_wr16(0x0010u, BSB_SOCK0_REG, CMK_COLLECTOR_PORT); /* Sn_DPORT */

	/* Serialize the struct into wire-format little-endian bytes,
	   field by field. This runs unconditionally (target is
	   big-endian SH-2; the collector expects LE per
	   coremark_result.h), so target and host always execute the
	   same serialization. Do not raw-copy the struct: on a
	   big-endian CPU that would put big-endian bytes on the wire. */
	put_le32(&buf[0],  r->magic);
	put_le32(&buf[4],  r->git_rev);
	put_le16(&buf[8],  r->crc);
	put_le16(&buf[10], 0);            /* _pad */
	put_le32(&buf[12], r->iterations);
	put_le32(&buf[16], r->cycles);
	put_le32(&buf[20], r->clk_hz);

	wr = w5500_rd16(0x0024u, BSB_SOCK0_REG);              /* Sn_TX_WR */
	w5500_wr(wr, BSB_SOCK0_TX, buf, 24);
	wr += 24u;
	w5500_wr16(0x0024u, BSB_SOCK0_REG, wr);

	w5500_wr8(0x0001u, BSB_SOCK0_REG, 0x20u);              /* Sn_CR=SEND */
	while (!(w5500_rd8(0x0002u, BSB_SOCK0_REG) & 0x10u))    /* SENDOK */
		;
	w5500_wr8(0x0002u, BSB_SOCK0_REG, 0x10u);               /* clear IR */
}

#ifndef HOST_TEST
static void delay_500ms(void)
{
	unsigned int start = CYCCNT;
	while ((CYCCNT - start) < 6000000u) /* ~500ms @ 12MHz */
		;
}
#endif

void report_result(struct coremark_result *r)
{
	eth_init();
	send_once(r);

#ifndef HOST_TEST
	for (;;) {
		delay_500ms();
		send_once(r);
	}
#endif
}
