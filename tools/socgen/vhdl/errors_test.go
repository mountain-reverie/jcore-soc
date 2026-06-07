package vhdl

import (
	"errors"
	"strings"
	"testing"
)

// TestParseErrorMessageFormat is a smoke test pinning the human-facing text of
// a parse diagnostic so it cannot silently regress.
func TestParseErrorMessageFormat(t *testing.T) {
	_, err := ParseFile(NewFileSet(), "t.vhd", []byte("package p is\nattribute foo of x : notakeyword is 1;\nend package;"))
	if err == nil {
		t.Fatal("expected a parse error")
	}
	var pe *ParseError
	if !errors.As(err, &pe) {
		t.Fatalf("expected *ParseError, got %T: %v", err, err)
	}
	// Pin the actual rendered format: "<filename>:<line>:<col>: <message>". The bad
	// attribute decl is on line 2, so the diagnostic must render with that prefix.
	got := pe.Error()
	if !strings.HasPrefix(got, "t.vhd:2:") {
		t.Fatalf("ParseError.Error() = %q, want prefix %q", got, "t.vhd:2:")
	}
	if !strings.HasSuffix(got, ": "+pe.Msg) {
		t.Fatalf("ParseError.Error() = %q, want it to end with %q", got, ": "+pe.Msg)
	}
	if pe.Pos.Filename != "t.vhd" {
		t.Fatalf("expected filename t.vhd, got %q", pe.Pos.Filename)
	}
}

// TestCPPErrorMessageFormat pins the CPPError human-facing text and unwrap.
func TestCPPErrorMessageFormat(t *testing.T) {
	inner := errors.New("boom")
	ce := &CPPError{Filename: "t.vhd", CPP: "gcc", Err: inner}
	if got, want := ce.Error(), "t.vhd: cpp (gcc) failed: boom"; got != want {
		t.Fatalf("CPPError.Error() = %q, want %q", got, want)
	}
	if !errors.Is(ce, inner) {
		t.Fatal("CPPError should unwrap to its inner error")
	}
}
