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

/* eth_tx @ 0xABCD1000 (DEVICE_ETH0_ADDR in board.h): 10BASE-T Manchester TX.
   Registers are byte-bus offsets within the 4 KiB-aligned device window. */
#define ETH_BASE   0xABCD1000u
#define ETH_DATA   (*(volatile unsigned int *)(ETH_BASE + 0x800u))
#define ETH_RSTPTR (*(volatile unsigned int *)(ETH_BASE + 0x804u))
#define ETH_LEN    (*(volatile unsigned int *)(ETH_BASE + 0x808u))
#define ETH_GO     (*(volatile unsigned int *)(ETH_BASE + 0x80Cu))
#define ETH_STATUS (*(volatile unsigned int *)(ETH_BASE + 0x810u))

/* IEEE 802.3 CRC-32 (reflected, poly 0xEDB88320), over dest..payload;
   the result is appended little-endian per 802.3. */
static unsigned int eth_crc32(const unsigned char *p, unsigned int n)
{
	unsigned int c = 0xFFFFFFFFu, i, j;
	for (i = 0; i < n; i++) {
		c ^= p[i];
		for (j = 0; j < 8; j++)
			c = (c >> 1) ^ (0xEDB88320u & (~((c & 1u) - 1u)));
	}
	return ~c;
}

/* Send a raw frame: 'body' = dest+src+type+payload (no preamble/FCS).
   FRAME_MAX sized for the short boot-time test frame, not a full 1518B
   Ethernet MTU -- the 2 KiB EBR boot has no room for a 1526-byte stack
   buffer. Bump if a larger test frame is ever needed. */
#define FRAME_MAX 64u
static void eth_send(const unsigned char *body, unsigned int body_len)
{
	unsigned char frame[FRAME_MAX];   /* preamble(7)+SFD(1)+body+FCS(4) */
	unsigned int i, n = 0, fcs;
	for (i = 0; i < 7; i++)
		frame[n++] = 0x55u;   /* preamble */
	frame[n++] = 0xD5u;           /* SFD */
	for (i = 0; i < body_len; i++)
		frame[n++] = body[i];
	fcs = eth_crc32(body, body_len);
	frame[n++] = fcs & 0xffu; frame[n++] = (fcs >> 8) & 0xffu;
	frame[n++] = (fcs >> 16) & 0xffu; frame[n++] = (fcs >> 24) & 0xffu;
	/* load buffer as 32-bit words (pad to a multiple of 4; big-endian store
	   => wire order) then transmit n bytes. */
	while (ETH_STATUS & 1u)
		;                      /* wait not busy */
	ETH_RSTPTR = 1u;
	for (i = 0; i < ((n + 3u) & ~3u); i += 4u)
		ETH_DATA = ((unsigned)frame[i] << 24) | ((unsigned)frame[i + 1] << 16)
			 | ((unsigned)frame[i + 2] << 8) | frame[i + 3];
	ETH_LEN = n;
	ETH_GO  = 1u;
	while (ETH_STATUS & 1u)
		;                      /* poll busy until done */
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

	/* build + transmit a broadcast test frame over eth_tx (10BASE-T).
	   Sent BEFORE the SPRAM sweep (below): the sweep covers 128 KB and takes
	   long enough in sim that pushing the TX past it risks running the frame
	   outside the testbench's --stop-time window. Sending here keeps it well
	   within the window and, completing well inside eth_tx_phy's NLP idle
	   period, avoids any ambiguity in the tb's Manchester decoder between
	   the real frame and an idle link pulse. */
	{
		static const unsigned char test_frame[] = {
			0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF,   /* dest: broadcast */
			0x02, 0x00, 0x00, 0x00, 0x00, 0x01,   /* src MAC */
			0x88, 0xB5,                            /* ethertype (experimental) */
			'J', '1'                               /* payload */
		};
		eth_send(test_frame, sizeof(test_frame));
		puts_uart("ETH TX\r\n");
	}

	spram_memtest();     /* proves all 128 KB read/write */

	/* visible heartbeat: toggle the LED forever (the banner above is what the
	   sim testbench checks; the blink is for on-hardware sanity). */
	for (;;) {
		volatile unsigned int d;
		GPIO_TOGGLE = 0x01u;
		for (d = 0; d < 200000u; d++)
			;
	}
}
