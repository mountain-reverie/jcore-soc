package main

import (
	"flag"
	"fmt"
	"net"
	"os"
	"time"
)

func main() {
	port := flag.Int("port", CollectorPort, "UDP port")
	timeout := flag.Duration("timeout", 60*time.Second, "receive timeout")
	expectCRC := flag.Uint("expect-crc", uint(ExpectedCRC), "expected CoreMark crcfinal (known-good, catches gcc miscompiles)")
	flag.Parse()

	addr := net.UDPAddr{Port: *port, IP: net.IPv4zero}
	conn, err := net.ListenUDP("udp", &addr)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer conn.Close()
	conn.SetReadDeadline(time.Now().Add(*timeout))

	buf := make([]byte, 64)
	for {
		n, _, err := conn.ReadFromUDP(buf)
		if err != nil {
			fmt.Fprintln(os.Stderr, "timeout: no result:", err)
			os.Exit(2)
		}
		r, perr := ParseResult(buf[:n])
		if perr != nil {
			continue // ignore stray traffic
		}
		if r.Cycles == 0 {
			continue // ignore zero-cycle (invalid) results
		}
		if verr := validate(r, uint16(*expectCRC)); verr != nil {
			// A real board result with the wrong CRC is a definite
			// miscompile signal, not stray traffic -- fail hard rather
			// than keep waiting for another datagram.
			fmt.Fprintln(os.Stderr, verr)
			os.Exit(3)
		}
		ips := float64(r.ClkHz) / float64(r.Cycles) * float64(r.Iterations)
		fmt.Printf("coremark git=%#x crc=%#x iterations=%d cycles=%d iters_per_sec=%.2f\n",
			r.GitRev, r.CRC, r.Iterations, r.Cycles, ips)
		os.Exit(0)
	}
}
