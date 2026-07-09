package main

import "testing"

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
