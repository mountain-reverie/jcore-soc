/* cosim_main.c -- minimal Task 8b GHDL cosim payload.
 *
 * Not CoreMark itself: exercises just enough of the real board firmware
 * path (start_time/stop_time -> portme_finish -> report_result's W5500
 * emitter) to prove flash-boot -> SPRAM -> CPU execution -> W5500 SPI
 * result emission end-to-end in a GHDL cosim, without paying the full
 * CoreMark iteration count's simulated-cycle cost.
 */
#include "core_portme.h"
#include "coremark_result.h"

extern void start_time(void);
extern void stop_time(void);
extern CORE_TICKS get_time(void);
extern void portme_finish(unsigned short crc, unsigned int iterations,
                           CORE_TICKS cycles);

int
main(void)
{
	volatile unsigned int busy;

	start_time();
	for (busy = 0; busy < 300; busy++)
		;
	stop_time();

	portme_finish(0xABCD, 1000, get_time());

	for (;;)
		;
}
