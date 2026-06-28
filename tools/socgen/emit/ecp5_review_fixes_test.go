package emit

import (
	"os"
	"strings"
	"testing"

	"github.com/j-core/jcore-soc/tools/socgen/board"
	"github.com/j-core/jcore-soc/tools/socgen/elaborate"
	"github.com/j-core/jcore-soc/tools/socgen/vhdl"
)

func contextHasUnisim(nodes []vhdl.Node) bool {
	for _, n := range nodes {
		if lc, ok := n.(*vhdl.LibraryClause); ok {
			for _, nm := range lc.Names {
				if nm == "unisim" {
					return true
				}
			}
		}
	}
	return false
}

// FIX 1: ecp5 padring context must omit the unisim library/use clause that the
// Xilinx path emits.
func TestPadringContextOmitsUnisimForEcp5(t *testing.T) {
	root := os.Getenv("JCORE_SOC_ROOT")
	if root == "" {
		t.Skip("JCORE_SOC_ROOT not set")
	}
	b, _ := board.Load(root, "mimas_v2", "")
	if b == nil || b.Design == nil {
		t.Fatal("board.Load mimas_v2 failed")
	}
	res, _ := elaborate.Elaborate(b)
	if res == nil {
		t.Fatal("elaborate failed")
	}
	res.Target = "spartan6"
	if !contextHasUnisim(padringContext(res)) {
		t.Errorf("spartan6 padring context must include unisim")
	}
	res.Target = "ecp5"
	if contextHasUnisim(padringContext(res)) {
		t.Errorf("ecp5 padring context must NOT include unisim")
	}
}

// FIX 2/3: differential and inverted-output ecp5 pads must error, not emit wrong
// hardware.
func TestEcp5DeferredPinShapesError(t *testing.T) {
	cases := []struct {
		name string
		pin  *elaborate.ResolvedPin
	}{
		{"differential", &elaborate.ResolvedPin{Net: "clk_p", Pad: "A1", Signal: "clk", Diff: "pos", PadDir: "in"}},
		{"inverted-output", &elaborate.ResolvedPin{Net: "rst_n", Pad: "B1", Out: "reset", OutInvert: true, PadDir: "out"}},
		{"bidirectional", &elaborate.ResolvedPin{Net: "io0", Pad: "C1", In: "io_i", Out: "io_o", PadDir: "inout"}},
	}
	for _, tc := range cases {
		res := &elaborate.Resolution{Target: "ecp5", Pins: []*elaborate.ResolvedPin{tc.pin}}
		if _, err := pinStatements(res); err == nil {
			t.Errorf("%s pad: expected an error, got nil", tc.name)
		}
	}
}

// FIX 4: a constant-output ecp5 pad is emitted (was previously a misleading
// "deferred" error).
func TestEcp5ConstantOutputPad(t *testing.T) {
	res := &elaborate.Resolution{
		Target: "ecp5",
		Pins:   []*elaborate.ResolvedPin{{Net: "vcc", Pad: "D1", OutConst: "'1'", PadDir: "out"}},
	}
	stmts, err := pinStatements(res)
	if err != nil {
		t.Fatalf("constant-output pad should not error: %v", err)
	}
	out := printStmts(stmts)
	if !strings.Contains(out, "pin_vcc <=") {
		t.Errorf("expected an assignment driving pin_vcc; got:\n%s", out)
	}
}
