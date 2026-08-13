/* uart_io_test.c -- host-C unit test for uart_io.c.
   Build+run: cc -DHOST_TEST -I. uart_io.c uart_io_test.c -o /tmp/uartiotest \
                 && /tmp/uartiotest
   Expected: prints "uart_io OK", exit 0. */
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include "uart_io.h"

static void check(int cond, const char *msg)
{
	if (!cond) {
		fprintf(stderr, "FAIL: %s\n", msg);
		exit(1);
	}
}

static void expect_tx(const char *want, const char *msg)
{
	check(uart_host_tx_n == (int)strlen(want), msg);
	check(memcmp(uart_host_tx, want, strlen(want)) == 0, msg);
	uart_host_tx_n = 0;
}

int main(void)
{
	uart_host_tx_n = 0;

	uart_puts("HI\r\n");
	expect_tx("HI\r\n", "uart_puts");

	uart_put_hex32(0x4b4d434au);
	expect_tx("0x4b4d434a", "hex32 full width");

	uart_put_hex32(0u);
	expect_tx("0x00000000", "hex32 zero is zero-padded");

	uart_put_hex32(0xd340u);
	expect_tx("0x0000d340", "hex32 pads narrow values");

	uart_put_dec32(0u);
	expect_tx("0", "dec32 zero");

	uart_put_dec32(1000u);
	expect_tx("1000", "dec32 no padding");

	uart_put_dec32(4294967295u);
	expect_tx("4294967295", "dec32 max");

	/* RX: empty until a byte is pushed, then readable exactly once. */
	check(uart_rx_ready() == 0, "rx_ready must be 0 when empty");
	uart_host_rx_push('g');
	check(uart_rx_ready() == 1, "rx_ready must be 1 after push");
	check(uart_getc() == 'g', "getc returns pushed byte");
	check(uart_rx_ready() == 0, "rx drains after getc");

	/* RX: push N bytes, read exactly N bytes, queue drains to empty. */
	uart_host_rx_push('A');
	uart_host_rx_push('B');
	uart_host_rx_push('C');
	check(uart_rx_ready() == 1, "rx_ready after multiple pushes");
	check(uart_getc() == 'A', "getc first byte");
	check(uart_getc() == 'B', "getc second byte");
	check(uart_getc() == 'C', "getc third byte");
	check(uart_rx_ready() == 0, "queue empty after draining all bytes");

	printf("uart_io OK\n");
	return 0;
}
