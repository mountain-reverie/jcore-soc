package elaborate

import (
	"os"
	"testing"

	"github.com/j-core/jcore-soc/tools/socgen/board"
)

func TestElaborateThreadsTarget(t *testing.T) {
	root := os.Getenv("JCORE_SOC_ROOT")
	if root == "" {
		t.Skip("JCORE_SOC_ROOT not set")
	}
	b, lerr := board.Load(root, "mimas_v2")
	if b == nil || b.Design == nil {
		t.Fatalf("board.Load: %v", lerr)
	}
	res, rerr := Elaborate(b)
	if rerr != nil {
		t.Logf("elaborate notes: %v", rerr)
	}
	if res == nil {
		t.Fatalf("Elaborate returned nil resolution")
	}
	if res.Target != "spartan6" {
		t.Errorf("res.Target = %q, want %q", res.Target, "spartan6")
	}
}
