/* eth_report_test.c -- host-C model unit test for eth_report.c.
   Build+run: cc -DHOST_TEST -I. eth_report.c eth_report_test.c \
                 -o /tmp/ethtest && /tmp/ethtest
   Expected: prints "eth_report OK", exit 0. */
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include "eth_report.h"

/* fake W5500 model state, defined non-static in eth_report.c under
   HOST_TEST. */
extern unsigned char hm_common[0x40];
extern unsigned char hm_sock0reg[0x40];
extern unsigned char hm_sock0tx[2048];

static void check(int cond, const char *msg)
{
	if (!cond) {
		fprintf(stderr, "FAIL: %s\n", msg);
		exit(1);
	}
}

int main(void)
{
	struct coremark_result r;
	unsigned char want_ip[4] = CMK_COLLECTOR_IP;
	unsigned int want_port = CMK_COLLECTOR_PORT;
	unsigned int txwr;

	eth_init();

	/* Sn_MR == UDP (0x02) */
	check(hm_sock0reg[0x00] == 0x02u, "Sn_MR != UDP");
	/* Sn_SR == SOCK_UDP (0x22) i.e. OPEN succeeded */
	check(hm_sock0reg[0x03] == 0x22u, "Sn_SR != SOCK_UDP (not OPEN)");

	r.magic      = CMK_MAGIC;
	r.git_rev    = 0x01020304u;
	r.crc        = 0xABCDu;
	r._pad       = 0;
	r.iterations = 1000u;         /* 0x3E8 */
	r.cycles     = 0x11223344u;
	r.clk_hz     = 12000000u;     /* 0x00B71B00 */

	report_result(&r);

	/* Sn_DIPR == collector IP */
	check(memcmp(&hm_sock0reg[0x0C], want_ip, 4) == 0,
	      "Sn_DIPR != collector IP");
	/* Sn_DPORT == collector port (big-endian on the wire) */
	check(hm_sock0reg[0x10] == (unsigned char)(want_port >> 8) &&
	      hm_sock0reg[0x11] == (unsigned char)(want_port & 0xffu),
	      "Sn_DPORT != collector port");

	/* TX buffer holds the exact 24 wire bytes, little-endian per
	   coremark_result.h, at the offset that was in Sn_TX_WR before the
	   send (i.e. at offset 0, since this is the first send). This
	   checks LITERAL wire bytes (not struct equality via memcmp against
	   the in-host struct) so it catches a raw-memcpy regression
	   regardless of host endianness. */
	txwr = 0;
	{
		static const unsigned char want_wire[24] = {
			0x4A, 0x43, 0x4D, 0x4B,   /* magic, LE */
			0x04, 0x03, 0x02, 0x01,   /* git_rev, LE */
			0xCD, 0xAB,               /* crc, LE */
			0x00, 0x00,               /* _pad */
			0xE8, 0x03, 0x00, 0x00,   /* iterations, LE */
			0x44, 0x33, 0x22, 0x11,   /* cycles, LE */
			0x00, 0x1B, 0xB7, 0x00,   /* clk_hz, LE */
		};
		check(memcmp(&hm_sock0tx[txwr], want_wire, 24) == 0,
		      "TX buffer does not hold the literal LE wire bytes");
		check(hm_sock0tx[txwr + 0] == 0x4Au, "magic byte0 != 0x4A");
	}

	/* Sn_TX_WR advanced by 24 (the wire size) */
	check(((unsigned int)hm_sock0reg[0x24] << 8 | hm_sock0reg[0x25]) ==
	      24u, "Sn_TX_WR not advanced by 24");

	/* SEND was issued and SENDOK was observed+cleared (IR cleared after
	   send_once's ack). */
	check((hm_sock0reg[0x02] & 0x10u) == 0u,
	      "Sn_IR.SENDOK not cleared after ack");

	printf("eth_report OK\n");
	return 0;
}
