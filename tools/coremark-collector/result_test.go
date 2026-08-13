package main

import (
	"strings"
	"testing"
)

func TestParseRecordGolden(t *testing.T) {
	lines := []string{
		"CMK MAGIC=0x4b4d434a",
		"CMK GITREV=0x01020304",
		"CMK CRC=0x0000d340",
		"CMK ITERATIONS=1000",
		"CMK CYCLES=6629",
		"CMK CLKHZ=12000000",
	}
	r, err := ParseRecord(lines)
	if err != nil {
		t.Fatalf("ParseRecord: %v", err)
	}
	if r.Magic != Magic {
		t.Errorf("Magic: got %#x, want %#x", r.Magic, Magic)
	}
	if r.GitRev != 0x01020304 {
		t.Errorf("GitRev: got %#x, want %#x", r.GitRev, 0x01020304)
	}
	if r.CRC != 0xd340 {
		t.Errorf("CRC: got %#x, want %#x", r.CRC, 0xd340)
	}
	if r.Iterations != 1000 {
		t.Errorf("Iterations: got %d, want %d", r.Iterations, 1000)
	}
	if r.Cycles != 6629 {
		t.Errorf("Cycles: got %d, want %d", r.Cycles, 6629)
	}
	if r.ClkHz != 12000000 {
		t.Errorf("ClkHz: got %d, want %d", r.ClkHz, 12000000)
	}
}

func TestParseRecordCRCNotTruncated(t *testing.T) {
	// CMK CRC=0x0000d340 must parse to exactly 0xd340, not truncated
	// (e.g. by taking only the low byte) and not sign-extended.
	lines := []string{
		"CMK MAGIC=0x4b4d434a",
		"CMK GITREV=0x01020304",
		"CMK CRC=0x0000d340",
		"CMK ITERATIONS=1000",
		"CMK CYCLES=6629",
		"CMK CLKHZ=12000000",
	}
	r, err := ParseRecord(lines)
	if err != nil {
		t.Fatalf("ParseRecord: %v", err)
	}
	if r.CRC != 0xd340 {
		t.Fatalf("CRC: got %#04x, want %#04x", r.CRC, 0xd340)
	}
}

func TestParseRecordMissingField(t *testing.T) {
	lines := []string{
		"CMK MAGIC=0x4b4d434a",
		"CMK GITREV=0x01020304",
		// CRC missing
		"CMK ITERATIONS=1000",
		"CMK CYCLES=6629",
		"CMK CLKHZ=12000000",
	}
	_, err := ParseRecord(lines)
	if err == nil {
		t.Fatal("expected error for missing CRC field")
	}
	if !strings.Contains(strings.ToUpper(err.Error()), "CRC") {
		t.Errorf("error should name the missing field CRC, got: %v", err)
	}
}

func TestParseRecordMalformedHex(t *testing.T) {
	lines := []string{
		"CMK MAGIC=0x4b4d434a",
		"CMK GITREV=0x01020304",
		"CMK CRC=0xZZZZ",
		"CMK ITERATIONS=1000",
		"CMK CYCLES=6629",
		"CMK CLKHZ=12000000",
	}
	if _, err := ParseRecord(lines); err == nil {
		t.Fatal("expected error for malformed hex value")
	}
}

func TestParseRecordUnknownFieldIgnored(t *testing.T) {
	lines := []string{
		"CMK MAGIC=0x4b4d434a",
		"CMK GITREV=0x01020304",
		"CMK CRC=0x0000d340",
		"CMK FOO=1",
		"CMK ITERATIONS=1000",
		"CMK CYCLES=6629",
		"CMK CLKHZ=12000000",
	}
	r, err := ParseRecord(lines)
	if err != nil {
		t.Fatalf("ParseRecord: %v", err)
	}
	if r.Cycles != 6629 {
		t.Errorf("Cycles: got %d, want %d", r.Cycles, 6629)
	}
}

func TestParseRecordInterleavedNoise(t *testing.T) {
	lines := []string{
		"boot: hello world",
		"CMK MAGIC=0x4b4d434a",
		"some debug noise here",
		"CMK GITREV=0x01020304",
		"CMK CRC=0x0000d340",
		"CMK ITERATIONS=1000",
		"more noise",
		"CMK CYCLES=6629",
		"CMK CLKHZ=12000000",
		"CMK DONE",
	}
	r, err := ParseRecord(lines)
	if err != nil {
		t.Fatalf("ParseRecord: %v", err)
	}
	if r.Cycles != 6629 {
		t.Errorf("Cycles: got %d, want %d", r.Cycles, 6629)
	}
}

func TestParseRecordZeroCyclesRejected(t *testing.T) {
	lines := []string{
		"CMK MAGIC=0x4b4d434a",
		"CMK GITREV=0x01020304",
		"CMK CRC=0x0000d340",
		"CMK ITERATIONS=1000",
		"CMK CYCLES=0",
		"CMK CLKHZ=12000000",
	}
	if _, err := ParseRecord(lines); err == nil {
		t.Fatal("expected error for Cycles=0")
	}
}

func TestValidateCRCMismatchRejected(t *testing.T) {
	// A well-formed, non-zero-cycle result whose CRC does not match the
	// known-good CoreMark crcfinal is a miscompile signal and must be
	// rejected by validate(), not silently accepted as a pass.
	r := Result{
		Magic:      Magic,
		GitRev:     0x1,
		CRC:        0xdead, // deliberately wrong
		Iterations: 1000,
		Cycles:     16,
		ClkHz:      100000000,
	}
	if err := validate(r, ExpectedCRC); err == nil {
		t.Fatal("expected CRC mismatch to be rejected, got nil error")
	}
}

func TestValidateCRCMatchAccepted(t *testing.T) {
	r := Result{
		Magic:      Magic,
		GitRev:     0x1,
		CRC:        ExpectedCRC,
		Iterations: 1000,
		Cycles:     16,
		ClkHz:      100000000,
	}
	if err := validate(r, ExpectedCRC); err != nil {
		t.Fatalf("expected matching CRC to be accepted, got: %v", err)
	}
}
