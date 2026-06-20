#ifndef BOARD_H
#define BOARD_H
#include <inttypes.h>

#define DRAM_BASE 0x10000000

/* Peripheral addresses — must match design.yaml */
#define DEVICE_GPIO_ADDR   0xabcd0000
#define DEVICE_AIC0_ADDR   0xabcd0040
#define DEVICE_UART0_ADDR  0xabcd0100
#define DEVICE_SPI2_ADDR   0xabcd0200   /* spi2: SD card */

/* uartlite registers (uartlitedb layout: 32-bit stores, a(3) selects reg) */
struct uartlite_regs {
  uint32_t rx;      /* +0x00 */
  uint32_t tx;      /* +0x04 */
  uint32_t status;  /* +0x08 */
  uint32_t ctrl;    /* +0x0C */
};
#define DEVICE_UART0 ((volatile struct uartlite_regs *) DEVICE_UART0_ADDR)

/* aic v1 registers */
struct aic_regs {
  uint32_t ctrl0;
  uint32_t brkadd;
  uint32_t ilevels;
  uint32_t ctrl1;
  uint32_t pit_throttle;
  uint32_t pit_counter;
  uint32_t clock_period;
  uint32_t ignore0;
  uint32_t rtc_sec_hi;
  uint32_t rtc_sec_lo;
  uint32_t rtc_nsec;
};
#define DEVICE_AIC0 ((volatile struct aic_regs *) DEVICE_AIC0_ADDR)

/* gpio2 registers */
struct gpio_regs {
  uint32_t value;
  uint32_t mask;
  uint32_t edge;
  uint32_t changes;
};
#define DEVICE_GPIO ((volatile struct gpio_regs *) DEVICE_GPIO_ADDR)

/* spi2 registers (a(2)=0 → ctrl, a(2)=1 → data) */
struct spi2_regs {
  uint32_t ctrl;  /* +0: bit0=cs[0], bit1=start/busy, bit2=cs[1], bit3=loop, bits31:27=speed */
  uint32_t data;  /* +4: bits7:0 = tx (write) / rx (read) */
};
#define DEVICE_SPI2 ((volatile struct spi2_regs *) DEVICE_SPI2_ADDR)

#endif /* BOARD_H */
