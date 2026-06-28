package board

import "testing"

func TestDesignPathForVariant(t *testing.T) {
	if got := designFileName(""); got != "design.yaml" {
		t.Errorf("empty variant: got %q", got)
	}
	if got := designFileName("j4-dual"); got != "design.j4-dual.yaml" {
		t.Errorf("variant: got %q", got)
	}
}
