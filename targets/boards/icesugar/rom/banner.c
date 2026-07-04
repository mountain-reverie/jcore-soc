/* J1 on iCESugar (iCE40 UP5K), EBR-only: emit a UART banner over uart0 and
   blink the RGB LED via gpio2 d_o(0). No SDRAM, no interrupts -- everything
   runs from on-chip inferred EBR. Kept tiny to fit the up5k EBR budget. */

/* uartlitedb @ 0xABCD0100: a(3)=0 selects data, a(3)=1 selects status.
   (Matches the ULX3S banner: byte stores would land in d(31:24) on the
   big-endian SH-2, so all UART writes are 32-bit.) */
#define UART_DATA   (*(volatile unsigned int *)0xABCD0100u)  /* a(3)=0 */
#define UART_STATUS (*(volatile unsigned int *)0xABCD0108u)  /* a(3)=1 */
#define TX_FULL     (1u << 3)

/* gpio2 @ 0xABCD0000: d_o -> LEDs (jcore,gpio2). */
#define GPIO_BASE   0xABCD0000u
#define GPIO_DATA   (*(volatile unsigned int *)(GPIO_BASE + 0x00u)) /* wr d_o */
#define GPIO_TOGGLE (*(volatile unsigned int *)(GPIO_BASE + 0x08u)) /* XOR d_o */

static void putc_uart(char c)
{
	while (UART_STATUS & TX_FULL)
		;
	UART_DATA = (unsigned int)(unsigned char)c;   /* 32-bit store, see above */
}

static void puts_uart(const char *p)
{
	while (*p)
		putc_uart(*p++);
}

/* eth @ 0xABCD1000 (DEVICE_ETH_ADDR in board.h): spi2 master driving a
   W5500 (WIZ850io PMOD) over SPI. spi2 register semantics (confirmed
   against components/misc/spi2.vhd):
     ETH_CTRL (+0x0), write: bit0 = cs(0) (idle high=1; write 0 to assert
       CS), bit1 = start_txn (write 1 to begin a byte transfer),
       bit2 = cs(1) (unused, W5500 is cs(0) only).
     ETH_CTRL (+0x0), read: bit0 = cs(0), bit1 = busy (1 while a byte
       transfer is in progress).
     ETH_DATA (+0x4), write: bits[7:0] = byte to shift out next transfer.
     ETH_DATA (+0x4), read: bits[7:0] = byte shifted in during the last
       transfer. */
#define ETH_BASE   0xABCD1000u
#define ETH_CTRL   (*(volatile unsigned int *)(ETH_BASE + 0x0u))
#define ETH_DATA   (*(volatile unsigned int *)(ETH_BASE + 0x4u))
#define ETH_CTRL_CS0    0x1u
#define ETH_CTRL_START  0x2u
#define ETH_CTRL_BUSY   0x2u

static void spi_assert(void)
{
	ETH_CTRL = 0u;                 /* cs(0) = 0: assert CS (idle high) */
}

static void spi_deassert(void)
{
	ETH_CTRL = ETH_CTRL_CS0;       /* cs(0) = 1: deassert CS */
}

/* One SPI byte transfer; CS must already be asserted. */
static unsigned char spi_byte(unsigned char txval)
{
	ETH_DATA = txval;
	ETH_CTRL = ETH_CTRL_START;     /* cs(0)=0, start_txn=1 */
	while (ETH_CTRL & ETH_CTRL_BUSY)
		;
	return (unsigned char)ETH_DATA;
}

/* W5500 common-block (BSB=0) register write, VDM (variable data length)
   mode: 3-byte header (addr_hi, addr_lo, control) then n data bytes, all
   under one CS assertion. control = 0x04 for a common-block write. */
static void w5500_write(unsigned int addr, const unsigned char *buf, int n)
{
	int i;
	spi_assert();
	spi_byte((unsigned char)(addr >> 8));
	spi_byte((unsigned char)(addr & 0xffu));
	spi_byte(0x04u);
	for (i = 0; i < n; i++)
		spi_byte(buf[i]);
	spi_deassert();
}

static void w5500_write8(unsigned int addr, unsigned char val)
{
	w5500_write(addr, &val, 1);
}

/* Reset the W5500 and program MAC/IP/subnet/gateway. Once these are set
   (and MR.PB, the ping-block bit, is left 0) the chip auto-answers ARP and
   ICMP echo entirely in hardware -- no CPU socket code needed. */
static void w5500_init_ping(void)
{
	static const unsigned char shar[6] = { 0x02, 0x00, 0x00, 0x00, 0x00, 0x01 };
	static const unsigned char sipr[4] = { 192, 168, 1, 10 };
	static const unsigned char subr[4] = { 255, 255, 255, 0 };
	static const unsigned char gar[4]  = { 192, 168, 1, 1 };
	volatile unsigned int d;

	w5500_write8(0x0000u, 0x80u);   /* MR.RST: software reset */
	for (d = 0; d < 2000u; d++)
		;                        /* let RST self-clear (datasheet: fast) */

	w5500_write(0x0009u, shar, 6);  /* SHAR: source MAC */
	w5500_write(0x000Fu, sipr, 4);  /* SIPR: source IP */
	w5500_write(0x0005u, subr, 4);  /* SUBR: subnet mask */
	w5500_write(0x0001u, gar, 4);   /* GAR: gateway */
}

#define SPRAM_BASE  0x10000000u
#define SPRAM_WORDS (128u*1024u/4u)   /* 32768 words */

extern unsigned int _spram_load[], _spram_start[], _spram_end[];

/* Runs from SPRAM (proves instruction fetch from SPRAM works). */
static void __attribute__((section(".spram"), noinline))
spram_routine(void)
{
	puts_uart("FROM SPRAM\r\n");
}

/* Write a marching pattern across all 128 KB and read it back. Bounded so the
   sim completes within the testbench window. */
static void spram_memtest(void)
{
	volatile unsigned int *p = (volatile unsigned int *)SPRAM_BASE;
	unsigned int i, bad = 0u;
	for (i = 0u; i < SPRAM_WORDS; i++) p[i] = i * 2654435761u;   /* Knuth hash */
	for (i = 0u; i < SPRAM_WORDS; i++) if (p[i] != i * 2654435761u) bad++;
	puts_uart(bad ? "SPRAM MEMTEST FAIL\r\n" : "SPRAM MEMTEST OK\r\n");
}

void main(void)
{
	puts_uart("J1 on iCESugar: hello\r\n");
	GPIO_DATA = 0x01u;            /* light LED via gpio2 d_o(0) */
	puts_uart("GPIO\r\n");

	/* copy the .spram routine (LMA in EBR) up to SPRAM, then execute it there */
	{
		unsigned int *dst = _spram_start, *src = _spram_load;
		while ((unsigned int)dst < (unsigned int)_spram_end) *dst++ = *src++;
	}
	spram_routine();     /* executes out of SPRAM -> prints "FROM SPRAM" */

	spram_memtest();     /* proves all 128 KB read/write */

	w5500_init_ping();   /* reset + program MAC/IP/subnet/gateway; the W5500
	                        then auto-answers ARP/ICMP entirely in hardware */
	puts_uart("W5500 INIT OK\r\n");

	/* visible heartbeat; nothing else to poll now the W5500 handles ARP/ICMP
	   on its own once MAC/IP are programmed. */
	{
		unsigned int hb = 0u;
		for (;;) {
			volatile unsigned int d;
			if (++hb >= 400u) {   /* ~0.5 s heartbeat at this spin length */
				hb = 0u;
				GPIO_TOGGLE = 0x01u;
			}
			for (d = 0; d < 1500u; d++)
				;
		}
	}
}
