/*
 * core_portme.h -- jcore/icesugar (SH-2, J1 CPU) bare-metal port.
 *
 * Modeled on the reference barebones port at
 * gcc-sh-monitor/coremark/barebones/core_portme.h, trimmed to what this
 * payload needs: no time.h, no printf, cycle-counter based timing.
 */
#ifndef CORE_PORTME_H
#define CORE_PORTME_H

#include <stdint.h>
#include <stddef.h>

/************************/
/* Data types and settings */
/************************/
#ifndef HAS_FLOAT
#define HAS_FLOAT 0
#endif
#ifndef HAS_TIME_H
#define HAS_TIME_H 0
#endif
#ifndef USE_CLOCK
#define USE_CLOCK 0
#endif
#ifndef HAS_STDIO
#define HAS_STDIO 0
#endif
#ifndef HAS_PRINTF
#define HAS_PRINTF 0
#endif

#ifndef COMPILER_VERSION
#ifdef __GNUC__
#define COMPILER_VERSION "GCC" __VERSION__
#else
#define COMPILER_VERSION "Please put compiler version here (e.g. gcc 4.1)"
#endif
#endif
#ifndef COMPILER_FLAGS
#define COMPILER_FLAGS "-O2 -m2"
#endif
#ifndef MEM_LOCATION
#define MEM_LOCATION "STATIC"
#endif

/* Data Types :
        To avoid compiler issues, define the data types that need to be used
   for 8b, 16b and 32b in <core_portme.h>.
*/
typedef signed short   ee_s16;
typedef unsigned short ee_u16;
typedef signed int     ee_s32;
typedef double         ee_f32;
typedef unsigned char  ee_u8;
typedef unsigned int   ee_u32;
typedef ee_u32         ee_ptr_int;
typedef size_t         ee_size_t;
#define NULL ((void *)0)

#define align_mem(x) (void *)(4 + (((ee_ptr_int)(x)-1) & ~3))

/* Configuration : CORE_TICKS
        Cycle-counter tick, read from the read-only cyccnt MMIO register.
 */
#define CORETIMETYPE ee_u32
typedef uint32_t CORE_TICKS;

/* cyccnt MMIO: read-only 32b free-running cycle counter (soc_gen generated
   board.h calls this DEVICE_CYCCNT_ADDR). Duplicated here so this port does
   not have to pull in the full generated board.h. */
#define CYCCNT_ADDR 0xabcd0200u
#define CYCCNT (*(volatile uint32_t *)CYCCNT_ADDR)

#ifndef SEED_METHOD
#define SEED_METHOD SEED_VOLATILE
#endif

#ifndef MEM_METHOD
#define MEM_METHOD MEM_STATIC
#endif

/* Configuration : MULTITHREAD
        Single context; this port has no parallel execution support. */
#ifndef MULTITHREAD
#define MULTITHREAD 1
#define USE_PTHREAD 0
#define USE_FORK    0
#define USE_SOCKET  0
#endif

/* No argc/argv on bare metal. */
#ifndef MAIN_HAS_NOARGC
#define MAIN_HAS_NOARGC 1
#endif

#ifndef MAIN_HAS_NORETURN
#define MAIN_HAS_NORETURN 0
#endif

/* Variable : default_num_contexts
        Not used for this simple port, must contain the value 1.
*/
extern ee_u32 default_num_contexts;

typedef struct CORE_PORTABLE_S
{
    ee_u8 portable_id;
} core_portable;

/* target specific init/fini */
void portable_init(core_portable *p, int *argc, char *argv[]);
void portable_fini(core_portable *p);

#if !defined(PROFILE_RUN) && !defined(PERFORMANCE_RUN) \
    && !defined(VALIDATION_RUN)
#if (TOTAL_DATA_SIZE == 1200)
#define PROFILE_RUN 1
#elif (TOTAL_DATA_SIZE == 2000)
#define PERFORMANCE_RUN 1
#else
#define VALIDATION_RUN 1
#endif
#endif

/* HAS_PRINTF is 0: ee_printf is still referenced by vendored core_main.c
   (the alias to printf() in coremark.h is only compiled in when
   HAS_PRINTF), so core_portme.c provides a no-op ee_printf() to satisfy the
   linker without pulling in any UART/console dependency. */
int ee_printf(const char *fmt, ...);

/* Result finalize hook: called once from vendor/core_main.c's fenced
   "emit over eth instead of printf" edit with CoreMark's own crcfinal,
   iteration count, and elapsed cycle count. Implemented in core_portme.c;
   packs struct coremark_result (coremark_result.h) and calls
   report_result(). */
#ifndef GIT_REV
#define GIT_REV 0
#endif
struct coremark_result;
void portme_finish(ee_u16 crc, ee_u32 iterations, CORE_TICKS cycles);
void report_result(struct coremark_result *r);

#endif /* CORE_PORTME_H */
