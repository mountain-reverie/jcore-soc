/* uart_io.h -- UART TX/RX + integer formatting shared by the iCESugar boot
   banner (rom/banner.c) and the CoreMark result emitter
   (rom/coremark/uart_report.c), so both speak the same console.

   uartlitedb @ 0xABCD0100: a(3)=0 selects data, a(3)=1 selects status.
   Status bits (components/uartlite/uart.vhm, status reg read):
     bit3 = tx.full, bit0 = rx_valid (not rx_empty).
   All accesses are 32-bit: a byte store would land in d(31:24) on the
   big-endian SH-2. */
#ifndef UART_IO_H
#define UART_IO_H

void uart_putc(char c);
void uart_puts(const char *s);

/* 1 when a received byte is waiting, 0 otherwise. Never blocks. */
int uart_rx_ready(void);

/* On target: blocks until a byte arrives, then returns it.
   On host (HOST_TEST): returns immediately if a byte is queued; aborts the
   test (exit 1) if the queue is empty — this ensures detect under-supplies
   of input (e.g., wait_for_go loops) rather than silently spinning on zeros. */
char uart_getc(void);

/* "0x" + exactly 8 lowercase hex digits, zero-padded. Fixed width so a host
   parser never has to guess. */
void uart_put_hex32(unsigned int v);

/* Shortest decimal form, no padding; "0" for zero. */
void uart_put_dec32(unsigned int v);

#ifdef HOST_TEST
/* Host-test capture/injection state (defined in uart_io.c under HOST_TEST). */
extern char uart_host_tx[4096];
extern int uart_host_tx_n;
void uart_host_rx_push(char c);
#endif

#endif
