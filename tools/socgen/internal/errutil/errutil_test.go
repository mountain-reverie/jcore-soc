package errutil

import (
	"errors"
	"testing"
)

func TestErrors(t *testing.T) {
	if got := Errors(nil); got != nil {
		t.Errorf("nil -> %v want nil", got)
	}
	single := errors.New("one")
	if got := Errors(single); len(got) != 1 || got[0] != single { //nolint:errorlint // intentional identity check: Errors must return the exact error, not a wrap
		t.Errorf("single -> %v", got)
	}
	a, b := errors.New("a"), errors.New("b")
	if got := Errors(errors.Join(a, b)); len(got) != 2 {
		t.Errorf("joined -> %d want 2", len(got))
	}
	// errors.Join drops nils
	if got := Errors(errors.Join(nil, a, nil)); len(got) != 1 {
		t.Errorf("joined-with-nils -> %d want 1", len(got))
	}
}
