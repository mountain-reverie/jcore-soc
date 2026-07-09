/* coremark_result.h -- shared board<->collector contract. Little-endian wire. */
#ifndef COREMARK_RESULT_H
#define COREMARK_RESULT_H
#include <stdint.h>

#define CMK_MAGIC         0x4B4D434Au   /* 'J','C','M','K' as LE u32 */
#define CMK_FLASH_BASE    0x00100000u
#define CMK_SPRAM_BASE    0x10000000u
#define CMK_CLK_HZ        12000000u
#define CMK_COLLECTOR_PORT 47000u

/* Board network identity, reused verbatim from banner.c's hardware-tested
   W5500 config (targets/boards/icesugar/rom/banner.c w5500_init_ping()) so
   the board keeps a single network identity across banner and coremark
   payloads. */
#define CMK_BOARD_MAC     {0x02,0x00,0x00,0x00,0x00,0x01}
#define CMK_BOARD_IP      {192,168,1,10}
#define CMK_SUBNET        {255,255,255,0}
#define CMK_GATEWAY       {192,168,1,1}
#define CMK_COLLECTOR_IP  {192,168,1,1}   /* runner host = gateway; adjust here only */

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
#endif
