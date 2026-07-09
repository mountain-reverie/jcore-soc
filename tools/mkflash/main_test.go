package main

import "testing"

func TestAssemblePadsToFlashBase(t *testing.T) {
	bit := make([]byte, 100)
	for i := range bit { bit[i] = 0xAA }
	pay := []byte{1, 2, 3, 4}
	out, err := assemble(bit, pay)
	if err != nil { t.Fatal(err) }
	if len(out) != flashBase+len(pay) { t.Fatalf("len %d", len(out)) }
	if out[0] != 0xAA { t.Fatal("bitstream not at 0") }
	for i := 100; i < flashBase; i++ {
		if out[i] != 0xFF { t.Fatalf("pad not 0xFF at %d", i) }
	}
	if out[flashBase] != 1 || out[flashBase+3] != 4 { t.Fatal("payload misplaced") }
}

func TestAssembleRejectsOversizeBitstream(t *testing.T) {
	if _, err := assemble(make([]byte, flashBase+1), []byte{0}); err == nil {
		t.Fatal("expected error: bitstream overruns FLASH_BASE")
	}
}
