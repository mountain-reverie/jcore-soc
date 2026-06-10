package generate

import (
	"errors"
	"testing"
)

// TestErrorMessages is the message-format smoke for GenerateError, so the
// human-facing strings (and the Unwrap wiring) don't silently regress.
func TestErrorMessages(t *testing.T) {
	withDetail := &GenerateError{Kind: ErrWrite, Name: "out", Detail: "permission denied"}
	if got, want := withDetail.Error(), `generate "out": write failed: permission denied`; got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
	noDetail := &GenerateError{Kind: ErrUnknownPlugin, Name: "bogus"}
	if got, want := noDetail.Error(), `generate "bogus": unknown plugin`; got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
	if !errors.Is(withDetail, ErrWrite) {
		t.Errorf("errors.Is(withDetail, ErrWrite) = false, want true")
	}
}
