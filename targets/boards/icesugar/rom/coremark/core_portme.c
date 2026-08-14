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
#include "uart_io.h"

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

/* Progress indication on the board's RGB LED (gpio0, active-high here -- the
   pads invert). A CoreMark run at 12 MHz takes minutes, so without this the
   board looks identical whether it is working or hung.
     amber (red+green) = timed run in progress
     green             = run finished
   Deliberately in start_time/stop_time rather than the report path so the
   indication brackets exactly the timed portion. */
#define LED_OFF   0x0u
#define LED_AMBER 0x3u   /* gpio_do(0)=red | gpio_do(1)=green */
#define LED_GREEN 0x2u

/* board.h cannot be included here: CoreMark's headers already pull in the
   ROM's local inttypes.h, and board.h's <inttypes.h> then redefines the fixed
   width types. gpio0's data register is addressed directly instead. */
#define GPIO0_VALUE (*(volatile unsigned int *)0xabcd0000u)

static void
led_set(unsigned v)
{
    GPIO0_VALUE = v;
}

/* Function : start_time
        Called right before starting the timed portion of the benchmark. */
void
start_time(void)
{
    led_set(LED_AMBER);
    GETMYTIME(&start_time_val);
}

/* Function : stop_time
        Called right after ending the timed portion of the benchmark. */
void
stop_time(void)
{
    GETMYTIME(&stop_time_val);
    led_set(LED_GREEN);
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

    /* Divide self-check. gcc turns / and % into libgcc __udivsi3/__sdivsi3,
       built from the SH-2 div0u/div1 step instructions, and CoreMark itself
       divides in five places -- so if the J1 gets these wrong every result is
       silently wrong. Printed in hex (decimal printing would itself divide).
       Reference values from the host: see divtest.c's twin. */
    {
        volatile unsigned int u_a = 1000000u, u_b = 10u;
        volatile unsigned int u_c = 0xFFFFFFFFu, u_d = 3u;
        volatile int          s_a = -1000000, s_b = 7;
        unsigned int i, sq = 0u, sr = 0u;

        uart_puts("\r\nDIV u1="); uart_put_hex32(u_a / u_b);
        uart_puts(" u2=");          uart_put_hex32(u_a % u_b);
        uart_puts(" u3=");          uart_put_hex32(u_c / u_d);
        uart_puts(" u4=");          uart_put_hex32(u_c % 7u);
        uart_puts("\r\nDIV s1="); uart_put_hex32((unsigned int)(s_a / s_b));
        uart_puts(" s2=");          uart_put_hex32((unsigned int)(s_a % s_b));
        for (i = 1u; i <= 1000u; i++) {
            sq += 0x12345678u / i;
            sr += 0x12345678u % i;
        }
        uart_puts("\r\nDIV sq="); uart_put_hex32(sq);
        uart_puts(" sr=");          uart_put_hex32(sr);
        uart_puts("\r\n");
    }

    /* Multiply self-check. gcc lowers a constant-divisor % into a
       reciprocal dmulu.l (32x32->64), so a wrong MULTIPLIER shows up as a
       wrong modulo. This board is the only one binding mult(ice40dsp) -- the
       SB_MAC16 DSP multiplier -- so check both halves explicitly. */
    {
        volatile unsigned int m1 = 0xFFFFFFFFu, m2 = 0xFFFFFFFFu;
        volatile unsigned int m3 = 0x12345678u, m4 = 0x9ABCDEF0u;
        volatile unsigned int m5 = 0x0000FFFFu, m6 = 0x00010001u;
        unsigned long long p;
        unsigned int i, acc = 0u;

        p = (unsigned long long)m1 * m2;
        uart_puts("MUL a_hi="); uart_put_hex32((unsigned int)(p >> 32));
        uart_puts(" a_lo=");    uart_put_hex32((unsigned int)p);
        p = (unsigned long long)m3 * m4;
        uart_puts("\r\nMUL b_hi="); uart_put_hex32((unsigned int)(p >> 32));
        uart_puts(" b_lo=");    uart_put_hex32((unsigned int)p);
        p = (unsigned long long)m5 * m6;
        uart_puts("\r\nMUL c_hi="); uart_put_hex32((unsigned int)(p >> 32));
        uart_puts(" c_lo=");    uart_put_hex32((unsigned int)p);
        for (i = 1u; i <= 1000u; i++)
            acc += (unsigned int)(((unsigned long long)0x9E3779B9u * i) >> 32);
        uart_puts("\r\nMUL acc="); uart_put_hex32(acc);
        uart_puts("\r\n");
    }

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
   struct coremark_result and hands it to report_result(); uart_report.c
   replaces the weak no-op below with the real UART text emitter. */
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

/* weak: overridden by uart_report.c's UART text emitter */
__attribute__((weak)) void
report_result(struct coremark_result *r)
{
    (void)r;
}
