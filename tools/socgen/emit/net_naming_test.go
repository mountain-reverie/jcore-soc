package emit

import (
	"os"
	"strings"
	"testing"

	"github.com/j-core/jcore-soc/tools/socgen/board"
	"github.com/j-core/jcore-soc/tools/socgen/elaborate"
)

func TestNetNamingMimasV2(t *testing.T) {
	root := os.Getenv("JCORE_SOC_ROOT")
	if root == "" {
		t.Skip("JCORE_SOC_ROOT not set")
	}
	b, lerr := board.Load(root, "mimas_v2", "")
	if b == nil || b.Design == nil {
		t.Fatalf("board.Load: %v", lerr)
	}
	res, rerr := elaborate.Elaborate(b)
	if rerr != nil {
		t.Logf("elaborate notes: %v", rerr)
	}
	soc, _ := SoC(res)
	pad, _ := PadRing(res)

	// soc.vhd: no extra signals; constant still used as an actual; clk_sys wired.
	for _, bad := range []string{"signal NULL_COPR_I", "\n    signal clk :"} {
		if strings.Contains(soc, bad) {
			t.Errorf("soc.vhd should not declare %q", bad)
		}
	}
	for _, want := range []string{"cpu0_copro_i => NULL_COPR_I", "clk => clk_sys"} {
		if !strings.Contains(soc, want) {
			t.Errorf("soc.vhd missing %q", want)
		}
	}

	// pad_ring.vhd: RST/reset_i gone; pll_rst present + wired.
	for _, bad := range []string{"signal RST", "signal reset_i"} {
		if strings.Contains(pad, bad) {
			t.Errorf("pad_ring.vhd should not declare %q", bad)
		}
	}
	for _, want := range []string{"signal pll_rst", "reset_i => pll_rst", "rst => reset"} {
		if !strings.Contains(pad, want) {
			t.Errorf("pad_ring.vhd missing %q", want)
		}
	}

	// soc.vhd signal-decl block byte-exact to golden (order + content).
	golden, gerr := os.ReadFile(root + "/targets/boards/mimas_v2/soc.vhd")
	if gerr != nil {
		t.Fatalf("read golden: %v", gerr)
	}
	if g, o := signalLines(string(golden)), signalLines(soc); g != o {
		t.Errorf("soc.vhd signal-decl block != golden:\n got:\n%s\nwant:\n%s", o, g)
	}
}

// signalLines returns the `    signal …` lines of a VHDL file, in order, joined.
func signalLines(s string) string {
	var b strings.Builder
	for _, ln := range strings.Split(s, "\n") {
		if strings.HasPrefix(ln, "    signal ") {
			b.WriteString(ln)
			b.WriteByte('\n')
		}
	}
	return b.String()
}
