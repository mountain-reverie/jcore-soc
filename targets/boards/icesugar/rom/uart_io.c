/* uart_io.c -- see uart_io.h. Two backends: real MMIO on the J1, and a
   capture buffer / injected byte queue under -DHOST_TEST so the formatting
   and RX-drain logic can be unit tested on the build host. */
#include "uart_io.h"

#ifdef HOST_TEST
#include <stdio.h>
#include <stdlib.h>

char uart_host_tx[4096];
int uart_host_tx_n;

static char host_rx[256];
static int host_rx_head;
static int host_rx_tail;

void uart_host_rx_push(char c)
{
	if (host_rx_tail < (int)sizeof(host_rx))
		host_rx[host_rx_tail++] = c;
}

void uart_putc(char c)
{
	if (uart_host_tx_n < (int)sizeof(uart_host_tx))
		uart_host_tx[uart_host_tx_n++] = c;
}

int uart_rx_ready(void)
{
	return host_rx_head < host_rx_tail;
}

char uart_getc(void)
{
	if (!uart_rx_ready()) {
		fprintf(stderr, "FATAL: uart_getc() called with empty RX queue; "
				"test supplied insufficient input\n");
		exit(1);
	}
	return host_rx[host_rx_head++];
}

#else /* target */

#include "board.h"

#define TX_FULL     (1u << 3)
#define RX_VALID    (1u << 0)

/* uartlitedb decodes on a(3): rx/tx (+0x0/+0x4) both select the data
   register and status/ctrl (+0x8/+0xc) both select the status register --
   a byte store would land in d(31:24) on the big-endian SH-2, so all
   accesses are 32-bit. */
void uart_putc(char c)
{
	while (DEVICE_UART0->status & TX_FULL)
		;
	DEVICE_UART0->tx = (unsigned int)(unsigned char)c;
}

int uart_rx_ready(void)
{
	return (DEVICE_UART0->status & RX_VALID) ? 1 : 0;
}

char uart_getc(void)
{
	while (!(DEVICE_UART0->status & RX_VALID))
		;
	return (char)(DEVICE_UART0->rx & 0xFFu);
}

#endif /* HOST_TEST */

void uart_puts(const char *s)
{
	while (*s)
		uart_putc(*s++);
}

void uart_put_hex32(unsigned int v)
{
	static const char digits[] = "0123456789abcdef";
	int shift;

	uart_puts("0x");
	for (shift = 28; shift >= 0; shift -= 4)
		uart_putc(digits[(v >> shift) & 0xFu]);
}

/* Division-free decimal conversion: repeated subtraction against a table of
   powers of ten, rather than v/10 and v%10.
   The obvious /10, %10 form makes gcc emit __udivsi3, and on this J1 the
   record stalled at exactly the first decimal field ("CMK ITERATIONS=") while
   every preceding hex field -- pure shifts and masks -- printed fine. Decimal
   output is a handful of values at the end of a benchmark run, so trading a
   library division for at most 9 compares per digit costs nothing here. */
void uart_put_dec32(unsigned int v)
{
	static const unsigned int pow10[10] = {
		1000000000u, 100000000u, 10000000u, 1000000u, 100000u,
		10000u, 1000u, 100u, 10u, 1u
	};
	int i;
	int started = 0;

	for (i = 0; i < 10; i++) {
		unsigned int digit = 0u;

		while (v >= pow10[i]) {
			v -= pow10[i];
			digit++;
		}
		/* suppress leading zeros, but always emit the units digit */
		if (digit != 0u || started || i == 9) {
			uart_putc((char)('0' + digit));
			started = 1;
		}
	}
}
