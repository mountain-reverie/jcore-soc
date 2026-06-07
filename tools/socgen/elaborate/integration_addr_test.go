package elaborate

import (
	"os"
	"strings"
	"testing"

	"github.com/j-core/jcore-soc/tools/socgen/board"
)

// mimas_v2's 4 memory-mapped devices (0xabcd0000/0040/00c0/0100) are valid and
// non-overlapping, so a full elaborate must produce NO address-validation errors.
func TestElaborateAddrMimasV2(t *testing.T) {
	root := os.Getenv("JCORE_SOC_ROOT")
	if root == "" {
		t.Skip("JCORE_SOC_ROOT not set")
	}
	b, _ := board.Load(root, "mimas_v2")
	if b.Design == nil || b.Library == nil {
		t.Skip("board.Load incomplete")
	}
	_, errs := Elaborate(b)
	for _, e := range errs {
		m := e.Error()
		if strings.Contains(m, "bits 31-28 must be 0xA") ||
			strings.Contains(m, "internal address range") ||
			strings.Contains(m, "memory regions overlap") {
			t.Errorf("unexpected address-validation error on mimas_v2: %v", e)
		}
	}
}
