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

int
main(void)
{
	volatile unsigned int busy;

	for (;;) {
		wait_for_go();

		start_time();
		for (busy = 0; busy < 300; busy++)
			;
		stop_time();

		portme_finish(0xD340, 1000, get_time());
	}
}
