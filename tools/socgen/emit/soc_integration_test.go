package emit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/j-core/jcore-soc/tools/socgen/board"
	"github.com/j-core/jcore-soc/tools/socgen/elaborate"
	"github.com/j-core/jcore-soc/tools/socgen/vhdl"
)

func TestSoCParityMimasV2(t *testing.T) {
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
	out, derr := SoC(res)
	if derr != nil {
		t.Logf("emit notes: %v", derr)
	}
	df, perr := vhdl.ParseFile(vhdl.NewFileSet(), "soc.vhd", []byte(out))
	if perr != nil {
		t.Fatalf("emitted soc.vhd does not re-parse: %v\n%s", perr, out)
	}

	var ent *vhdl.EntityDecl
	insts := map[string]*vhdl.InstantiationStmt{}
	for _, u := range df.Units {
		switch n := u.(type) {
		case *vhdl.EntityDecl:
			ent = n
		case *vhdl.ArchitectureBody:
			for _, s := range n.Stmts {
				if i, ok := s.(*vhdl.InstantiationStmt); ok {
					insts[i.Label] = i
				}
			}
		}
	}
	if ent == nil {
		t.Fatal("no soc entity emitted")
	}

	// soc entity ports == golden soc.vhd's 16 (name:dir:type).
	got := emittedEntityPortsTyped(ent)
	want := goldenEntityPortsTyped(t, filepath.Join(root, "targets/boards/mimas_v2/soc.vhd"))
	if len(want) == 0 {
		t.Fatal("golden soc.vhd has no entity ports — parse/path problem")
	}
	if !equalStringsDE(got, want) {
		t.Errorf("soc entity ports mismatch (%d vs golden %d):\n got=%v\n want=%v", len(got), len(want), got, want)
	}

	// top-entity instances with the right kind + the devices instance.
	if c := insts["cpus"]; c == nil || c.UnitKind != vhdl.CONFIGURATION {
		t.Errorf("cpus must be a configuration instance: %+v", c)
	}
	for _, te := range []string{"ddr_ctrl", "ddr_ram_mux", "fpga_reboot"} {
		if insts[te] == nil {
			t.Errorf("top-entity %q not instantiated", te)
		}
	}
	if d := insts["devices"]; d == nil || d.Unit != "work.devices" || d.Arch != "impl" {
		t.Errorf("devices instance missing/wrong: %+v", d)
	}

	// zero-out: a known record signal zeros with the golden aggregate (normalized).
	norm := func(s string) string { return strings.ReplaceAll(strings.ToLower(s), " ", "") }
	if !strings.Contains(norm(out), norm("cpu1_event_i <= (en => '0', cmd => INTERRUPT")) {
		t.Errorf("zero-out for cpu1_event_i not as expected:\n%s", out)
	}

	t.Logf("mimas_v2 soc entity: %d ports (golden %d); insts=%d", len(got), len(want), len(insts))
}
