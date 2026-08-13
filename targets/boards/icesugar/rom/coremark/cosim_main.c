/* cosim_main.c -- minimal GHDL cosim payload.
 *
 * Not CoreMark itself: exercises just enough of the real board firmware
 * path (CMK READY -> wait for 'g' -> start_time/stop_time -> portme_finish
 * -> uart_report.c's text emitter) to prove flash-boot -> SPRAM -> CPU
 * execution -> UART result emission end-to-end, without paying the full
 * CoreMark iteration count's simulated-cycle cost.
 */
#include "core_portme.h"
#include "coremark_result.h"
#include "uart_io.h"

extern void start_time(void);
extern void stop_time(void);
extern CORE_TICKS get_time(void);
extern void portme_finish(unsigned short crc, unsigned int iterations,
                           CORE_TICKS cycles);

/* 0xD340 stands in for CoreMark's crcfinal -- the cosim proves the plumbing
   (wait_for_go -> timing -> portme_finish -> uart_report.c's emitter), not
   the CRC; only coremark.bin computes the real value. */
#define COSIM_FAKE_CRC 0xD340u

int
main(void)
{
	volatile unsigned int busy;

	/* One run per reset, matching vendor/core_main.c's shipping control
	   flow: wait_for_go() runs exactly once, then the program parks. A
	   for(;;) loop here would exercise a control-flow shape the real
	   firmware never takes, so this gate wouldn't catch the divergence. */
	wait_for_go();

	start_time();
	for (busy = 0; busy < 300; busy++)
		;
	stop_time();

	portme_finish(COSIM_FAKE_CRC, 1000, get_time());

	for (;;)
		;
}
