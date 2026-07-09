/* eth_report.h -- W5500 UDP driver for the CoreMark result emitter
   (iCESugar/J1). See eth_report.c for the SPI/W5500 register layer. */
#ifndef ETH_REPORT_H
#define ETH_REPORT_H

#include "coremark_result.h"

/* Reset + program the W5500 and open socket 0 in UDP mode. Idempotent via
   an internal static guard -- safe to call more than once. */
void eth_init(void);

/* STRONG override of core_portme.c's weak no-op: send the 24-byte result
   struct as a UDP datagram to CMK_COLLECTOR_IP:CMK_COLLECTOR_PORT, then
   (outside HOST_TEST) loop forever re-sending every ~500ms. Never returns
   under normal (non-HOST_TEST) build. */
void report_result(struct coremark_result *r);

#endif
