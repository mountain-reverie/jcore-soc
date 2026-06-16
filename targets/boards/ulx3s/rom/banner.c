/* J2 ULX3S M1b: banner + SDRAM memory test over uartlitedb (32-bit accesses). */
#define UART_DATA   (*(volatile unsigned int *)0xABCD0100u)  /* a(3)=0 */
#define UART_STATUS (*(volatile unsigned int *)0xABCD0108u)  /* a(3)=1 */
#define TX_FULL     (1u << 3)

#define SDRAM ((volatile unsigned int *)0x10000000u)  /* DEV_DDR base */

/* AIC v1 @ 0xABCD0040: interrupt controller + RTC + PIT (jcore,aic1). */
#define AIC_BASE  0xABCD0040u
#define AIC_CTRL  (*(volatile unsigned int *)(AIC_BASE + 0x00u)) /* pit_enable/testvect */
#define AIC_PIT   (*(volatile unsigned int *)(AIC_BASE + 0x10u)) /* PIT throttle (period) */
#define AIC_RTCNS (*(volatile unsigned int *)(AIC_BASE + 0x28u)) /* RTC nanoseconds */
#define PIT_VECNUM 0x40u    /* vector-table slot for the PIT interrupt */
#define PIT_LEVEL  0xFu     /* event level; SR I-bits (0) are below it -> accepted */

volatile unsigned int g_ticks;        /* incremented by the PIT interrupt */

/* Called by the _irq_entry trampoline (start.S). M2a: the PIT is the only
   enabled source, so every interrupt is a timer tick. */
void irq_dispatch(void)
{
	g_ticks++;
}

static void aic_init_pit(void)
{
	/* Throttle (period in 40ns ticks) must be written while pit_enable=0.
	   2500 ticks = 100us = ~10 kHz (fast so a few ticks pass quickly in sim). */
	AIC_PIT  = 2500u;
	/* control: pit_enable(d26)=1, testvect(d23:12) = (level<<8)|vecnum
	   (event_info: bits 11:8 = level, 7:0 = vector). */
	AIC_CTRL = (1u << 26) | (((PIT_LEVEL << 8) | PIT_VECNUM) << 12);
}

static void putc_uart(char c)
{
	while (UART_STATUS & TX_FULL)
		;
	/* Must be a 32-bit store: uartlitedb reads d(7:0); a byte store on the
	   big-endian SH2 would land the char in d(31:24) and transmit 0x00. */
	UART_DATA = (unsigned int)(unsigned char)c;
}

static void puts_uart(const char *p)
{
	while (*p)
		putc_uart(*p++);
}

/* Write/read 4 patterns at distinct word/line/row offsets in SDRAM. dcache is
   bypassed (write-through), so the stores reach SDRAM and the loads see them. */
static int sdram_test(void)
{
	static const unsigned int pat[4] =
		{0xDEADBEEFu, 0x12345678u, 0xA5A5A5A5u, 0xFFFFFFFFu};
	const unsigned int off[4] = {0u, 1u, 8u, 0x400u}; /* word, +word, +line, +row */
	int i;
	for (i = 0; i < 4; i++)
		SDRAM[off[i]] = pat[i];
	for (i = 0; i < 4; i++)
		if (SDRAM[off[i]] != pat[i])
			return 0;
	return 1;
}

/* Linked at VMA 0x10000000 (SDRAM) but stored (LMA) in BRAM; run_from_sdram
   copies it up, then calls it. `used` keeps it despite being reached only after
   the copy. It may call back into BRAM .text (puts_uart) normally. */
__attribute__((section(".sdram"), noinline, used))
static void sdram_routine(void)
{
	puts_uart("FROM SDRAM\r\n");
}

extern unsigned int sdram_start[], sdram_end[], sdram_load[];

static void run_from_sdram(void)
{
	unsigned int *dst = sdram_start;
	unsigned int *src = sdram_load;
	/* compare as integers: sdram_start/sdram_end are distinct linker symbols, so
	   pointer-< between them is UB in C even though the addresses are flat. */
	while ((unsigned int)dst < (unsigned int)sdram_end)  /* write-through -> SDRAM */
		*dst++ = *src++;
	sdram_routine();          /* cold icache fetches it from SDRAM */
}

void main(void)
{
	puts_uart("J2 on ULX3S: hello\r\n");
	if (sdram_test())
		puts_uart("SDRAM TEST PASS\r\n");
	else
		puts_uart("SDRAM TEST FAIL\r\n");
	run_from_sdram();

	/* arm the PIT and wait for a few periodic interrupts (proves the AIC
	   delivers a vectored interrupt to the J2) */
	aic_init_pit();
	{
		unsigned int t0 = g_ticks;
		while (g_ticks < t0 + 3)
			;
		puts_uart("TICK\r\n");
	}
	if (AIC_RTCNS != 0u)        /* RTC is counting -> clocksource alive */
		puts_uart("RTC\r\n");

	for (;;)
		;
}
