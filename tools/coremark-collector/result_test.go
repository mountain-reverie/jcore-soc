package main

import (
	"net"
	"testing"
	"time"
)

func TestParseGolden(t *testing.T) {
	b := make([]byte, 24)
	// magic 'J','C','M','K'
	copy(b, []byte{0x4A, 0x43, 0x4D, 0x4B})
	b[16], b[17], b[18], b[19] = 0x10, 0x00, 0x00, 0x00 // cycles = 16
	r, err := ParseResult(b)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if r.Magic != Magic {
		t.Fatalf("magic %#x", r.Magic)
	}
	if r.Cycles != 16 {
		t.Fatalf("cycles %d", r.Cycles)
	}
}

func TestParseShort(t *testing.T) {
	if _, err := ParseResult(make([]byte, 10)); err == nil {
		t.Fatal("expected short-packet error")
	}
}

func TestParseBadMagic(t *testing.T) {
	if _, err := ParseResult(make([]byte, 24)); err == nil {
		t.Fatal("expected bad-magic error")
	}
}

func TestListenerReceivesGolden(t *testing.T) {
	// Bind listener on ephemeral port
	addr := net.UDPAddr{Port: 0, IP: net.IPv4(127, 0, 0, 1)}
	listener, err := net.ListenUDP("udp", &addr)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer listener.Close()

	// Set a 1-second read deadline
	listener.SetReadDeadline(time.Now().Add(1 * time.Second))

	// Create a golden packet
	goldenPacket := make([]byte, 24)
	copy(goldenPacket, []byte{0x4A, 0x43, 0x4D, 0x4B})                      // magic
	goldenPacket[4], goldenPacket[5], goldenPacket[6], goldenPacket[7] = 0xAB, 0xCD, 0xEF, 0x00 // GitRev
	goldenPacket[8], goldenPacket[9] = 0x12, 0x34                            // CRC
	goldenPacket[12], goldenPacket[13], goldenPacket[14], goldenPacket[15] = 0x78, 0x56, 0x34, 0x12 // Iterations
	goldenPacket[16], goldenPacket[17], goldenPacket[18], goldenPacket[19] = 0x10, 0x00, 0x00, 0x00 // Cycles = 16
	goldenPacket[20], goldenPacket[21], goldenPacket[22], goldenPacket[23] = 0x00, 0xE1, 0xF5, 0x05 // ClkHz = 100MHz

	// Send packet from another UDP socket
	sender, err := net.DialUDP("udp", nil, listener.LocalAddr().(*net.UDPAddr))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer sender.Close()

	_, err = sender.Write(goldenPacket)
	if err != nil {
		t.Fatalf("write: %v", err)
	}

	// Read from listener
	buf := make([]byte, 64)
	n, _, err := listener.ReadFromUDP(buf)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	// Parse the received packet
	r, err := ParseResult(buf[:n])
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	// Verify parsed result
	if r.Magic != Magic {
		t.Errorf("magic: got %#x, want %#x", r.Magic, Magic)
	}
	if r.GitRev != 0x00EFCDAB {
		t.Errorf("GitRev: got %#x, want %#x", r.GitRev, 0x00EFCDAB)
	}
	if r.CRC != 0x3412 {
		t.Errorf("CRC: got %#x, want %#x", r.CRC, 0x3412)
	}
	if r.Iterations != 0x12345678 {
		t.Errorf("Iterations: got %#x, want %#x", r.Iterations, 0x12345678)
	}
	if r.Cycles != 16 {
		t.Errorf("Cycles: got %d, want %d", r.Cycles, 16)
	}
	if r.ClkHz != 0x05f5e100 {
		t.Errorf("ClkHz: got %#x, want %#x", r.ClkHz, 0x05f5e100)
	}
}
