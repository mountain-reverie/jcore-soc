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

void uart_put_dec32(unsigned int v)
{
	/* buf[10] sized for exactly 10 decimal digits (4294967295 = 2^32-1). */
	char buf[10];
	int n = 0;

	if (v == 0u) {
		uart_putc('0');
		return;
	}
	while (v > 0u) {
		buf[n++] = (char)('0' + (v % 10u));
		v /= 10u;
	}
	while (n > 0)
		uart_putc(buf[--n]);
}
