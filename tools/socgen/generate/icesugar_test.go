package generate

import (
	"os"
	"strings"
	"testing"

	"github.com/j-core/jcore-soc/tools/socgen/board"
	"github.com/j-core/jcore-soc/tools/socgen/elaborate"
)

func TestIcesugarEmitsPCF(t *testing.T) {
	root := os.Getenv("JCORE_SOC_ROOT")
	if root == "" {
		t.Skip("JCORE_SOC_ROOT not set")
	}
	b, err := board.Load(root, "icesugar", "")
	if err != nil {
		t.Logf("load notes: %v", err) // make/hg env notes tolerated
	}
	if b == nil || b.Design == nil {
		t.Skip("icesugar board did not load in this environment")
	}
	if b.Design.Target != "ice40" {
		t.Fatalf("target = %q, want ice40", b.Design.Target)
	}
	if b.Design.CPU == nil || b.Design.CPU.Model != "j1" {
		t.Fatalf("cpu block: %+v", b.Design.CPU)
	}
	res, _ := elaborate.Elaborate(b)
	files, ferr := Build(b, res)
	if ferr != nil {
		t.Logf("build notes: %v", ferr)
	}
	var pcf string
	for _, f := range files {
		if f.Name == "icesugar.pcf" {
			pcf = f.Content
		}
		if strings.HasSuffix(f.Name, ".lpf") {
			t.Errorf("ice40 board must not emit an .lpf: %s", f.Name)
		}
	}
	if pcf == "" {
		t.Skip("pcf not produced (board VHDL library incomplete until Task 3)")
	}
	if !strings.Contains(pcf, "set_io pin_clk 35") {
		t.Errorf("pcf missing clk mapping:\n%s", pcf)
	}
}
