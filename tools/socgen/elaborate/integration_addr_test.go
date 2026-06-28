package elaborate

import (
	"errors"
	"os"
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
	b, _ := board.Load(root, "mimas_v2", "")
	if b.Design == nil || b.Library == nil {
		t.Skip("board.Load incomplete")
	}
	_, err := Elaborate(b)
	if errors.Is(err, ErrBadRegion) || errors.Is(err, ErrOverSpec) || errors.Is(err, ErrAddrOverlap) {
		t.Errorf("unexpected address-validation error on mimas_v2: %v", err)
	}
}
