/*
 * core_portme.c -- jcore/icesugar (SH-2, J1 CPU) bare-metal port.
 *
 * Modeled on the reference barebones port at
 * gcc-sh-monitor/coremark/barebones/core_portme.c. The only substantive
 * change from that reference: barebones_clock() reads the cyccnt MMIO
 * cycle counter instead of a host/RTC timer.
 */
#include "coremark.h"
#include "core_portme.h"
#include "coremark_result.h"

/* Porting : Seed values
        SEED_METHOD == SEED_VOLATILE (core_portme.h): core_util.c's
   get_seed_32() reads these extern volatiles. Mirrors the reference
   barebones core_portme.c seed table 1:1 (TOTAL_DATA_SIZE == 2000 here ->
   PERFORMANCE_RUN, per the #if ladder in core_portme.h).
*/
#if VALIDATION_RUN
volatile ee_s32 seed1_volatile = 0x3415;
volatile ee_s32 seed2_volatile = 0x3415;
volatile ee_s32 seed3_volatile = 0x66;
#endif
#if PERFORMANCE_RUN
volatile ee_s32 seed1_volatile = 0x0;
volatile ee_s32 seed2_volatile = 0x0;
volatile ee_s32 seed3_volatile = 0x66;
#endif
#if PROFILE_RUN
volatile ee_s32 seed1_volatile = 0x8;
volatile ee_s32 seed2_volatile = 0x8;
volatile ee_s32 seed3_volatile = 0x8;
#endif
/* seed4 == iteration count. This port has no way to auto-calibrate an
   ~10s run (no host loop, and burning real hardware cycles on a
   calibration pre-pass is wasteful), so ITERATIONS is a fixed build-time
   macro (see Makefile: -DITERATIONS=1000) rather than 0 (auto). */
volatile ee_s32 seed4_volatile = ITERATIONS;
volatile ee_s32 seed5_volatile = 0;

/* Porting : Timing functions
        cyccnt is a read-only, free-running 32b cycle counter MMIO register
   (Task 3). barebones_clock() returns its current value; start_time/
   stop_time/get_time follow the same pattern as the reference barebones
   port (static start/stop values, get_time returns the delta).
*/
static CORETIMETYPE
barebones_clock(void)
{
    return CYCCNT;
}

#define GETMYTIME(_t)              (*_t = barebones_clock())
#define MYTIMEDIFF(fin, ini)       ((fin) - (ini))
#define TIMER_RES_DIVIDER          1
#define SAMPLE_TIME_IMPLEMENTATION 1
#define EE_TICKS_PER_SEC           (CMK_CLK_HZ / TIMER_RES_DIVIDER)

static CORETIMETYPE start_time_val, stop_time_val;

/* Function : start_time
        Called right before starting the timed portion of the benchmark. */
void
start_time(void)
{
    GETMYTIME(&start_time_val);
}

/* Function : stop_time
        Called right after ending the timed portion of the benchmark. */
void
stop_time(void)
{
    GETMYTIME(&stop_time_val);
}

/* Function : get_time
        Return the elapsed ticks (cyccnt delta) between start_time() and
   stop_time(). */
CORE_TICKS
get_time(void)
{
    CORE_TICKS elapsed
        = (CORE_TICKS)(MYTIMEDIFF(stop_time_val, start_time_val));
    return elapsed;
}

/* Function : time_in_secs
        Convert a tick delta (cyccnt counts) to seconds, using CMK_CLK_HZ
   from the board<->collector contract header. */
secs_ret
time_in_secs(CORE_TICKS ticks)
{
    secs_ret retval = ((secs_ret)ticks) / (secs_ret)EE_TICKS_PER_SEC;
    return retval;
}

ee_u32 default_num_contexts = 1;

/* Function : portable_init
        Target specific initialization code. Nothing needed on this port:
   no UART, no heap, no RTC. */
void
portable_init(core_portable *p, int *argc, char *argv[])
{
    (void)argc;
    (void)argv;
    p->portable_id = 1;
}

/* Function : portable_fini
        Target specific final code. */
void
portable_fini(core_portable *p)
{
    p->portable_id = 0;
}

/* HAS_PRINTF is 0 on this port: no UART/console. core_main.c still
   references ee_printf() unconditionally (only the printf-alias macro in
   coremark.h is gated on HAS_PRINTF), so provide a no-op to satisfy the
   linker. */
int
ee_printf(const char *fmt, ...)
{
    (void)fmt;
    return 0;
}

/* gcc -O2 lowers some unreachable/UB paths in vendored code (e.g. the
   mergesort helper in core_list_join.c) to a call to abort(); this is
   compiler-runtime plumbing, not something CoreMark ever expects to hit in
   a valid run. Provide a bare-metal stub so it links. */
void
abort(void)
{
    for (;;)
        ;
}

/* Function : portme_finish
        Called once from core_main.c's iterate/report path (see the fenced
   edit in vendor/core_main.c) with the values CoreMark itself considers
   authoritative: crcfinal (results[0].crc, the CRC CoreMark's own
   correctness check is based on), the iteration count actually run, and
   the elapsed cycle count for the timed region. Packs the wire-format
   struct coremark_result and hands it to report_result(); Task 6 replaces
   the weak no-op below with the real W5500 emitter. */
void
portme_finish(ee_u16 crc, ee_u32 iterations, CORE_TICKS cycles)
{
    struct coremark_result r;

    r.magic      = CMK_MAGIC;
    r.git_rev    = GIT_REV;
    r.crc        = crc;
    r._pad       = 0;
    r.iterations = iterations;
    r.cycles     = cycles;
    r.clk_hz     = CMK_CLK_HZ;

    report_result(&r);
}

/* weak: overridden by the eth_report.c emitter landing in Task 6 */
__attribute__((weak)) void
report_result(struct coremark_result *r)
{
    (void)r;
}
