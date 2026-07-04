/* J1 on iCESugar (iCE40 UP5K), EBR-only: emit a UART banner over uart0 and
   blink the RGB LED via gpio2 d_o(0). No SDRAM, no interrupts -- everything
   runs from on-chip inferred EBR. Kept tiny to fit the up5k EBR budget. */

/* uartlitedb @ 0xABCD0100: a(3)=0 selects data, a(3)=1 selects status.
   (Matches the ULX3S banner: byte stores would land in d(31:24) on the
   big-endian SH-2, so all UART writes are 32-bit.) */
#define UART_DATA   (*(volatile unsigned int *)0xABCD0100u)  /* a(3)=0 */
#define UART_STATUS (*(volatile unsigned int *)0xABCD0108u)  /* a(3)=1 */
#define TX_FULL     (1u << 3)

/* gpio2 @ 0xABCD0000: d_o -> LEDs (jcore,gpio2). */
#define GPIO_BASE   0xABCD0000u
#define GPIO_DATA   (*(volatile unsigned int *)(GPIO_BASE + 0x00u)) /* wr d_o */
#define GPIO_TOGGLE (*(volatile unsigned int *)(GPIO_BASE + 0x08u)) /* XOR d_o */

static void putc_uart(char c)
{
	while (UART_STATUS & TX_FULL)
		;
	UART_DATA = (unsigned int)(unsigned char)c;   /* 32-bit store, see above */
}

static void puts_uart(const char *p)
{
	while (*p)
		putc_uart(*p++);
}

/* eth_tx @ 0xABCD1000 (DEVICE_ETH0_ADDR in board.h): 10BASE-T Manchester TX.
   Registers are byte-bus offsets within the 4 KiB-aligned device window. */
#define ETH_BASE   0xABCD1000u
#define ETH_DATA   (*(volatile unsigned int *)(ETH_BASE + 0x800u))
#define ETH_RSTPTR (*(volatile unsigned int *)(ETH_BASE + 0x804u))
#define ETH_LEN    (*(volatile unsigned int *)(ETH_BASE + 0x808u))
#define ETH_GO     (*(volatile unsigned int *)(ETH_BASE + 0x80Cu))
#define ETH_STATUS (*(volatile unsigned int *)(ETH_BASE + 0x810u))

/* IEEE 802.3 CRC-32 (reflected, poly 0xEDB88320), over dest..payload;
   the result is appended little-endian per 802.3. */
static unsigned int eth_crc32(const unsigned char *p, unsigned int n)
{
	unsigned int c = 0xFFFFFFFFu, i, j;
	for (i = 0; i < n; i++) {
		c ^= p[i];
		for (j = 0; j < 8; j++)
			c = (c >> 1) ^ (0xEDB88320u & (~((c & 1u) - 1u)));
	}
	return ~c;
}

/* Send a raw frame: 'body' = dest+src+type+payload (no preamble/FCS).
   FRAME_MAX sized to cover the boot-time test frame plus a standard ICMP
   echo (14 eth + 20 IP + 8 ICMP hdr + 56 default ping payload = 98 B body,
   +8 preamble/SFD +4 FCS = 110 B), not a full 1518B Ethernet MTU -- the
   2 KiB EBR boot has no room for a 1526-byte stack buffer. Bump if a
   larger frame is ever needed. */
#define FRAME_MAX 144u
static void eth_send(const unsigned char *body, unsigned int body_len)
{
	unsigned char frame[FRAME_MAX];   /* preamble(7)+SFD(1)+body+FCS(4) */
	unsigned int i, n = 0, fcs;
	for (i = 0; i < 7; i++)
		frame[n++] = 0x55u;   /* preamble */
	frame[n++] = 0xD5u;           /* SFD */
	for (i = 0; i < body_len; i++)
		frame[n++] = body[i];
	fcs = eth_crc32(body, body_len);
	frame[n++] = fcs & 0xffu; frame[n++] = (fcs >> 8) & 0xffu;
	frame[n++] = (fcs >> 16) & 0xffu; frame[n++] = (fcs >> 24) & 0xffu;
	/* load buffer as 32-bit words (pad to a multiple of 4; big-endian store
	   => wire order) then transmit n bytes. */
	while (ETH_STATUS & 1u)
		;                      /* wait not busy */
	ETH_RSTPTR = 1u;
	for (i = 0; i < ((n + 3u) & ~3u); i += 4u)
		ETH_DATA = ((unsigned)frame[i] << 24) | ((unsigned)frame[i + 1] << 16)
			 | ((unsigned)frame[i + 2] << 8) | frame[i + 3];
	ETH_LEN = n;
	ETH_GO  = 1u;
	while (ETH_STATUS & 1u)
		;                      /* poll busy until done */
}

/* eth_rx registers (Task 5), same 4 KiB device window as eth_tx above. */
#define ETH_RX_STATUS (*(volatile unsigned int *)(ETH_BASE + 0x900u)) /* bit0 ready, bit1 overrun */
#define ETH_RX_LEN    (*(volatile unsigned int *)(ETH_BASE + 0x904u))
#define ETH_RX_DATA   (*(volatile unsigned int *)(ETH_BASE + 0x908u)) /* auto-inc word, big-endian */
#define ETH_RX_ACK    (*(volatile unsigned int *)(ETH_BASE + 0x90Cu)) /* wr bit0=1: release + rewind ptr */

/* Read the pending frame (if any) into buf (up to max bytes). Returns the
   received length, or 0 if no frame is ready. Each ETH_RX_DATA access here
   is its own C volatile load -> its own bus cycle, satisfying the hardware's
   "one bus cycle between reads" requirement; do not hand-unroll these. */
static unsigned int eth_recv(unsigned char *buf, unsigned int max)
{
	unsigned int n, i, w;
	if (!(ETH_RX_STATUS & 1u))
		return 0;
	n = ETH_RX_LEN;
	if (n > max)
		n = max;
	for (i = 0; i < n; i += 4) {
		w = ETH_RX_DATA;
		buf[i] = w >> 24;
		if (i + 1 < n) buf[i + 1] = w >> 16;
		if (i + 2 < n) buf[i + 2] = w >> 8;
		if (i + 3 < n) buf[i + 3] = w;
	}
	ETH_RX_ACK = 1u;   /* release the buffer + rewind read pointer for next frame */
	return n;
}

/* Our identity for the ARP/ICMP responder. */
static const unsigned char OUR_MAC[6] = { 0x02, 0x00, 0x00, 0x00, 0x00, 0x01 };
static const unsigned char OUR_IP[4]  = { 192, 168, 1, 10 };

/* 16-bit one's-complement checksum (RFC 1071) over an arbitrary byte range,
   with an optional running sum to fold in (used to add the pseudo header or
   to redo just part of a buffer). p need not be aligned or even length. */
static unsigned int cksum_add(const unsigned char *p, unsigned int n, unsigned int sum)
{
	unsigned int i;
	for (i = 0; i + 1 < n; i += 2)
		sum += ((unsigned int)p[i] << 8) | p[i + 1];
	if (i < n)
		sum += (unsigned int)p[i] << 8;   /* odd trailing byte, high half */
	return sum;
}

static unsigned short cksum_fold(unsigned int sum)
{
	while (sum >> 16)
		sum = (sum & 0xffffu) + (sum >> 16);
	return (unsigned short)~sum;
}

static unsigned short ip_cksum(const unsigned char *hdr, unsigned int hlen)
{
	return cksum_fold(cksum_add(hdr, hlen, 0));
}

static void put16(unsigned char *p, unsigned int v)
{
	p[0] = (unsigned char)(v >> 8);
	p[1] = (unsigned char)v;
}

static unsigned int get16(const unsigned char *p)
{
	return ((unsigned int)p[0] << 8) | p[1];
}

/* Handle one received frame: 'frame' starts at dest MAC (no preamble/SFD,
   the hardware strips those), 'len' includes the trailing 4-byte FCS.
   Answers ARP requests for OUR_IP and ICMP echo requests to OUR_IP. */
static void eth_handle(const unsigned char *frame, unsigned int len)
{
	unsigned int ethertype;

	if (len < 18)
		return;   /* too short to hold even an Ethernet header + FCS */

	/* Optional FCS check: CRC-32 over dest..payload (len-4 bytes) must match
	   the trailing 4 bytes (wire order = little-endian of the CRC result). */
	{
		unsigned int fcs = eth_crc32(frame, len - 4);
		unsigned int rx_fcs = (unsigned int)frame[len - 4]
			| ((unsigned int)frame[len - 3] << 8)
			| ((unsigned int)frame[len - 2] << 16)
			| ((unsigned int)frame[len - 1] << 24);
		if (fcs != rx_fcs)
			return;
	}

	ethertype = get16(frame + 12);

	/* ---- ARP request -> ARP reply ---- */
	if (ethertype == 0x0806u && len >= 4 + 42) {
		unsigned int opcode = get16(frame + 20);
		const unsigned char *tpa = frame + 38;   /* ARP target protocol addr */
		if (opcode == 1 &&
		    tpa[0] == OUR_IP[0] && tpa[1] == OUR_IP[1] &&
		    tpa[2] == OUR_IP[2] && tpa[3] == OUR_IP[3]) {
			unsigned char reply[42];
			const unsigned char *req_mac = frame + 6;    /* ARP sender HA */
			const unsigned char *req_ip  = frame + 28;   /* ARP sender PA */
			unsigned int i;

#ifdef ETH_PAIR_TEST
			putc_uart('Y');
#endif
			for (i = 0; i < 6; i++) reply[i] = req_mac[i];        /* eth dst */
			for (i = 0; i < 6; i++) reply[6 + i] = OUR_MAC[i];    /* eth src */
			put16(reply + 12, 0x0806u);                            /* ethertype */

			put16(reply + 14, 1);       /* htype = Ethernet */
			put16(reply + 16, 0x0800u); /* ptype = IPv4 */
			reply[18] = 6;              /* hlen */
			reply[19] = 4;              /* plen */
			put16(reply + 20, 2);       /* opcode = reply */
			for (i = 0; i < 6; i++) reply[22 + i] = OUR_MAC[i];   /* sender HA */
			for (i = 0; i < 4; i++) reply[28 + i] = OUR_IP[i];    /* sender PA */
			for (i = 0; i < 6; i++) reply[32 + i] = req_mac[i];   /* target HA */
			for (i = 0; i < 4; i++) reply[38 + i] = req_ip[i];    /* target PA */

			eth_send(reply, 42);
		}
		return;
	}

	/* ---- ICMP echo request -> echo reply ---- */
	if (ethertype == 0x0800u && len >= 4 + 34) {
		unsigned int ihl = (frame[14] & 0x0fu) * 4u;    /* IPv4 IHL in bytes */
		unsigned int ip_proto = frame[23];
		unsigned int body_len = len - 4;                /* frame w/o FCS */
		if (ihl >= 20 && 14 + ihl + 8 <= body_len && ip_proto == 1 &&
		    frame[14 + ihl] == 8 /* ICMP echo request */ &&
		    frame[30] == OUR_IP[0] && frame[31] == OUR_IP[1] &&
		    frame[32] == OUR_IP[2] && frame[33] == OUR_IP[3]) {
			/* eth_send() adds preamble(8)+FCS(4) on top of body_len, and its
			   internal frame[] is FRAME_MAX bytes -- cap here so we never
			   hand it something it can't hold. */
			unsigned char pkt[FRAME_MAX - 12u];
			unsigned int icmp_off = 14 + ihl;
			unsigned int icmp_len = body_len - icmp_off;
			unsigned int i, sum;

			if (body_len > sizeof(pkt))
				return;   /* ping payload too large for our scratch/eth_send buffers */

			for (i = 0; i < body_len; i++)
				pkt[i] = frame[i];

			/* swap eth addrs */
			for (i = 0; i < 6; i++) { pkt[i] = frame[6 + i]; pkt[6 + i] = OUR_MAC[i]; }
			/* swap IP addrs */
			for (i = 0; i < 4; i++) {
				pkt[26 + i] = frame[30 + i];   /* new src = old dst (us) */
				pkt[30 + i] = frame[26 + i];   /* new dst = old src (requester) */
			}
			/* ICMP: type 0 (echo reply), code unchanged, recompute checksum */
			pkt[icmp_off] = 0;                 /* type = echo reply */
			pkt[icmp_off + 1] = frame[icmp_off + 1]; /* code, unchanged */
			put16(pkt + icmp_off + 2, 0);       /* zero checksum before recompute */
			sum = cksum_add(pkt + icmp_off, icmp_len, 0);
			put16(pkt + icmp_off + 2, cksum_fold(sum));

			/* IPv4 header checksum: zero field, recompute over header only */
			put16(pkt + 24, 0);
			put16(pkt + 24, ip_cksum(pkt + 14, ihl));

			eth_send(pkt, body_len);
		}
		return;
	}
}

#define SPRAM_BASE  0x10000000u
#define SPRAM_WORDS (128u*1024u/4u)   /* 32768 words */

extern unsigned int _spram_load[], _spram_start[], _spram_end[];

/* Runs from SPRAM (proves instruction fetch from SPRAM works). */
static void __attribute__((section(".spram"), noinline))
spram_routine(void)
{
	puts_uart("FROM SPRAM\r\n");
}

/* Write a marching pattern across all 128 KB and read it back. Bounded so the
   sim completes within the testbench window. */
static void spram_memtest(void)
{
	volatile unsigned int *p = (volatile unsigned int *)SPRAM_BASE;
	unsigned int i, bad = 0u;
	for (i = 0u; i < SPRAM_WORDS; i++) p[i] = i * 2654435761u;   /* Knuth hash */
	for (i = 0u; i < SPRAM_WORDS; i++) if (p[i] != i * 2654435761u) bad++;
	puts_uart(bad ? "SPRAM MEMTEST FAIL\r\n" : "SPRAM MEMTEST OK\r\n");
}

/* Build + send a 42-byte ARP REQUEST for target_ip (gratuitous ARP when
   target_ip == OUR_IP: used by the two-iCESugar cross-connected pair test
   to kick off an RX->reply round trip without waiting on the 128 KB SPRAM
   memtest). Mirrors the ARP reply layout in eth_handle() above. */
static void send_arp_request(const unsigned char target_ip[4])
{
	unsigned char req[42];
	unsigned int i;

	for (i = 0; i < 6; i++) req[i] = 0xFFu;               /* eth dest: broadcast */
	for (i = 0; i < 6; i++) req[6 + i] = OUR_MAC[i];      /* eth src */
	put16(req + 12, 0x0806u);                              /* ethertype: ARP */

	put16(req + 14, 1);       /* htype = Ethernet */
	put16(req + 16, 0x0800u); /* ptype = IPv4 */
	req[18] = 6;               /* hlen */
	req[19] = 4;               /* plen */
	put16(req + 20, 1);       /* opcode = request */
	for (i = 0; i < 6; i++) req[22 + i] = OUR_MAC[i];     /* sender HA */
	for (i = 0; i < 4; i++) req[28 + i] = OUR_IP[i];      /* sender PA */
	for (i = 0; i < 6; i++) req[32 + i] = 0x00u;          /* target HA: unknown */
	for (i = 0; i < 4; i++) req[38 + i] = target_ip[i];   /* target PA */

	eth_send(req, 42);
}

/* Static (not on-stack): the boot stack is tiny, and 256 B would be a
   sizable chunk of it. Sized for ARP (42 B) or a small ping (~98 B) with
   headroom; eth_recv() truncates anything larger. */
static unsigned char rxbuf[256];

void main(void)
{
	puts_uart("J1 on iCESugar: hello\r\n");
	GPIO_DATA = 0x01u;            /* light LED via gpio2 d_o(0) */
	puts_uart("GPIO\r\n");

	/* copy the .spram routine (LMA in EBR) up to SPRAM, then execute it there */
	{
		unsigned int *dst = _spram_start, *src = _spram_load;
		while ((unsigned int)dst < (unsigned int)_spram_end) *dst++ = *src++;
	}
	spram_routine();     /* executes out of SPRAM -> prints "FROM SPRAM" */

	/* build + transmit a broadcast test frame over eth_tx (10BASE-T).
	   Sent BEFORE the SPRAM sweep (below): the sweep covers 128 KB and takes
	   long enough in sim that pushing the TX past it risks running the frame
	   outside the testbench's --stop-time window. Sending here keeps it well
	   within the window and, completing well inside eth_tx_phy's NLP idle
	   period, avoids any ambiguity in the tb's Manchester decoder between
	   the real frame and an idle link pulse. */
#ifndef ETH_PAIR_TEST
	/* In the two-iCESugar pair test this experimental test frame is omitted:
	   each node's eth_rx holds only a single frame, so sending BOTH this
	   frame and the gratuitous ARP request back-to-back would overrun the
	   peer's RX buffer and lose the ARP request before its poll loop reads
	   it. The pair test therefore puts exactly one frame (the ARP request)
	   on each wire -- the same one-frame-in pattern the single-SoC RX path
	   is proven good against. The default build is unaffected. */
	{
		static const unsigned char test_frame[] = {
			0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF,   /* dest: broadcast */
			0x02, 0x00, 0x00, 0x00, 0x00, 0x01,   /* src MAC */
			0x88, 0xB5,                            /* ethertype (experimental) */
			'J', '1'                               /* payload */
		};
		eth_send(test_frame, sizeof(test_frame));
		puts_uart("ETH TX\r\n");
	}
#endif

#ifndef ETH_PAIR_TEST
	spram_memtest();     /* proves all 128 KB read/write */
#endif

#ifdef ETH_PAIR_TEST
	/* Gratuitous ARP for OUR_IP: the peer iCESugar runs the same pairtest
	   image (same OUR_IP), so its eth_handle() responder answers this,
	   giving an RX->reply round trip without the 128 KB SPRAM memtest's
	   sim-time cost. The infinite poll loop below still answers the peer's
	   own request in the other direction. */
	send_arp_request(OUR_IP);
	puts_uart("ARP REQ\r\n");
#endif

	/* visible heartbeat + eth RX poll. The eth poll must stay responsive
	   (a received ARP/ICMP frame is answered on the very next iteration), so
	   the poll runs every loop with only a short spin between iterations; the
	   LED heartbeat is decoupled onto a slow counter so it still toggles at a
	   human-visible rate. (An earlier single ~100 ms busy-wait per iteration
	   made the responder poll only ~10x/second, which pushed the RX->reply
	   round-trip past the end-to-end sim's watchdog window.) */
	{
		unsigned int hb = 0u;
#ifdef ETH_PAIR_TEST
		unsigned int arpc = 0u;
#endif
		for (;;) {
			volatile unsigned int d;
			unsigned int n = eth_recv(rxbuf, sizeof(rxbuf));
			if (n) {
#ifdef ETH_PAIR_TEST
				putc_uart('R');
#endif
				eth_handle(rxbuf, n);
			}
#ifdef ETH_PAIR_TEST
			/* Re-send the gratuitous ARP periodically: the peer may not have
			   reached its RX poll loop when our first (boot-time) request went
			   out, so retry until it answers (bounded by the tb watchdog). */
			if (++arpc >= 40u) {
				arpc = 0u;
				send_arp_request(OUR_IP);
			}
#endif
			if (++hb >= 400u) {   /* ~0.5 s heartbeat at this spin length */
				hb = 0u;
				GPIO_TOGGLE = 0x01u;
			}
			for (d = 0; d < 1500u; d++)
				;
		}
	}
}
