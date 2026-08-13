/* uart_report_test.c -- host-C unit test for uart_report.c's text emitter.
   Build+run: cc -DHOST_TEST -I. -I.. ../uart_io.c uart_report.c \
                 uart_report_test.c -o /tmp/uartreporttest \
                 && /tmp/uartreporttest
   Expected: prints "uart_report OK", exit 0. */
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include "uart_io.h"
#include "coremark_result.h"

void report_result(struct coremark_result *r);

static void check(int cond, const char *msg)
{
	if (!cond) {
		fprintf(stderr, "FAIL: %s\n", msg);
		exit(1);
	}
}

int main(void)
{
	struct coremark_result r;
	static const char want[] =
		"CMK MAGIC=0x4b4d434a\r\n"
		"CMK GITREV=0x01020304\r\n"
		"CMK CRC=0x0000d340\r\n"
		"CMK ITERATIONS=1000\r\n"
		"CMK CYCLES=12345\r\n"
		"CMK CLKHZ=12000000\r\n"
		"CMK DONE\r\n";

	r.magic      = CMK_MAGIC;
	r.git_rev    = 0x01020304u;
	r.crc        = 0xd340u;
	r._pad       = 0u;
	r.iterations = 1000u;
	r.cycles     = 12345u;
	r.clk_hz     = CMK_CLK_HZ;

	uart_host_tx_n = 0;
	report_result(&r);

	check(uart_host_tx_n == (int)strlen(want), "emitted length mismatch");
	check(memcmp(uart_host_tx, want, strlen(want)) == 0, "emitted text mismatch");

	/* report_result must RETURN on the host build (no forever loop). */
	printf("uart_report OK\n");
	return 0;
}
