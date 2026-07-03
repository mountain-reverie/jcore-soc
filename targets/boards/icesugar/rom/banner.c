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

	/* visible heartbeat: toggle the LED forever (the banner above is what the
	   sim testbench checks; the blink is for on-hardware sanity). */
	for (;;) {
		volatile unsigned int d;
		GPIO_TOGGLE = 0x01u;
		for (d = 0; d < 200000u; d++)
			;
	}
}
