/* coremark_result.h -- internal result contract between core_portme.c's
   portme_finish() and uart_report.c's emitter. The host-facing contract is
   the "CMK key=value" text uart_report.c prints, not this struct. */
#ifndef COREMARK_RESULT_H
#define COREMARK_RESULT_H
#include <stdint.h>

#define CMK_MAGIC         0x4B4D434Au   /* 'J','C','M','K' as LE u32 */
#define CMK_FLASH_BASE    0x00100000u
#define CMK_SPRAM_BASE    0x10000000u
#define CMK_CLK_HZ        12000000u

struct coremark_result {
  uint32_t magic;
  uint32_t git_rev;
  uint16_t crc;
  uint16_t _pad;
  uint32_t iterations;
  uint32_t cycles;
  uint32_t clk_hz;
} __attribute__((packed));

_Static_assert(sizeof(struct coremark_result) == 24, "result must be 24 bytes");

/* Print "CMK READY" and block until the host sends 'g'; other bytes are
   discarded. Defined in rom/coremark/uart_report.c -- declared here (not in
   uart_io.h) because it is a CoreMark-level protocol symbol, only linked
   into images that pull in the rom/coremark tree, not every uart_io.c consumer
   (e.g. rom/banner.c links uart_io.c without uart_report.c). */
void wait_for_go(void);

#endif
