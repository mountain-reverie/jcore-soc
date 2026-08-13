/* uart_report.c -- STRONG override of core_portme.c's weak report_result()
   no-op. Emits the CoreMark result as human-readable "CMK key=value" lines
   over uart0, terminated by "CMK DONE".

   Replaces the former W5500/UDP emitter: the iCESugar's iCELink USB link
   carries a serial port that stays connected while the bitstream is flashed,
   so one UART is both the console and the result channel. The framing tells
   the host where the record ends, so unlike the UDP emitter there is no
   forever-resend loop. */
#include "uart_io.h"
#include "coremark_result.h"

void
report_result(struct coremark_result *r)
{
	uart_puts("CMK MAGIC=");
	uart_put_hex32(r->magic);
	uart_puts("\r\nCMK GITREV=");
	uart_put_hex32(r->git_rev);
	uart_puts("\r\nCMK CRC=");
	uart_put_hex32((unsigned int)r->crc);
	uart_puts("\r\nCMK ITERATIONS=");
	uart_put_dec32(r->iterations);
	uart_puts("\r\nCMK CYCLES=");
	uart_put_dec32((unsigned int)r->cycles);
	uart_puts("\r\nCMK CLKHZ=");
	uart_put_dec32(r->clk_hz);
	uart_puts("\r\nCMK DONE\r\n");
}
