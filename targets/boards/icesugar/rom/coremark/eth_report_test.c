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
	r.git_rev    = 0xdeadbeefu;
	r.crc        = 0x1234u;
	r._pad       = 0;
	r.iterations = 1000u;
	r.cycles     = 987654u;
	r.clk_hz     = CMK_CLK_HZ;

	report_result(&r);

	/* Sn_DIPR == collector IP */
	check(memcmp(&hm_sock0reg[0x0C], want_ip, 4) == 0,
	      "Sn_DIPR != collector IP");
	/* Sn_DPORT == collector port (big-endian on the wire) */
	check(hm_sock0reg[0x10] == (unsigned char)(want_port >> 8) &&
	      hm_sock0reg[0x11] == (unsigned char)(want_port & 0xffu),
	      "Sn_DPORT != collector port");

	/* TX buffer holds the exact 24 struct bytes at the offset that was
	   in Sn_TX_WR before the send (i.e. at offset 0, since this is the
	   first send). */
	txwr = 0;
	check(memcmp(&hm_sock0tx[txwr], &r, sizeof(r)) == 0,
	      "TX buffer does not hold the struct bytes");

	/* Sn_TX_WR advanced by sizeof(r) */
	check(((unsigned int)hm_sock0reg[0x24] << 8 | hm_sock0reg[0x25]) ==
	      sizeof(r), "Sn_TX_WR not advanced by struct size");

	/* SEND was issued and SENDOK was observed+cleared (IR cleared after
	   send_once's ack). */
	check((hm_sock0reg[0x02] & 0x10u) == 0u,
	      "Sn_IR.SENDOK not cleared after ack");

	printf("eth_report OK\n");
	return 0;
}
