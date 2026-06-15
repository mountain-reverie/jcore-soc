/* J2 ULX3S M0 banner: write a string over uartlitedb using 32-bit accesses. */
#define UART_DATA   (*(volatile unsigned int *)0xABCD0100u)  /* a(3)=0 */
#define UART_STATUS (*(volatile unsigned int *)0xABCD0108u)  /* a(3)=1 */
#define TX_FULL     (1u << 3)

static void putc_uart(char c)
{
	while (UART_STATUS & TX_FULL)
		;
	UART_DATA = (unsigned int)(unsigned char)c;
}

void main(void)
{
	static const char msg[] = "J2 on ULX3S: hello\r\n";
	const char *p;
	for (;;) {
		for (p = msg; *p; p++)
			putc_uart(*p);
	}
}
