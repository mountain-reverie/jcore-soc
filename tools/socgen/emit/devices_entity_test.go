package emit

import (
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/j-core/jcore-soc/tools/socgen/board"
	"github.com/j-core/jcore-soc/tools/socgen/elaborate"
	"github.com/j-core/jcore-soc/tools/socgen/vhdl"
)

// goldenEntityPortsTyped returns the sole entity's ports as sorted "name:dir:typemark".
func goldenEntityPortsTyped(t *testing.T, path string) []string {
	t.Helper()
	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	df, perr := vhdl.ParseFile(vhdl.NewFileSet(), filepath.Base(path), src)
	if perr != nil {
		t.Fatalf("parse %s: %v", path, perr)
	}
	var out []string
	for _, u := range df.Units {
		if e, ok := u.(*vhdl.EntityDecl); ok {
			for _, p := range e.Ports {
				d := p.Mode
				if d == "" {
					d = "in"
				}
				for _, n := range p.Names {
					out = append(out, n+":"+d+":"+p.SubtypeMark)
				}
			}
		}
	}
	sort.Strings(out)
	return out
}

func emittedEntityPortsTyped(ent *vhdl.EntityDecl) []string {
	var out []string
	for _, p := range ent.Ports {
		d := p.Mode
		if d == "" {
			d = "in"
		}
		for _, n := range p.Names {
			out = append(out, n+":"+d+":"+p.SubtypeMark)
		}
	}
	sort.Strings(out)
	return out
}

func TestDevicesEntityParityMimasV2(t *testing.T) {
	root := os.Getenv("JCORE_SOC_ROOT")
	if root == "" {
		t.Skip("JCORE_SOC_ROOT not set")
	}
	b, lerr := board.Load(root, "mimas_v2")
	if b == nil || b.Design == nil {
		t.Fatalf("board.Load: %v", lerr)
	}
	res, rerr := elaborate.Elaborate(b)
	if rerr != nil {
		t.Logf("elaborate notes: %v", rerr)
	}
	out, derr := Devices(res)
	if derr != nil {
		t.Logf("emit notes: %v", derr)
	}
	df, perr := vhdl.ParseFile(vhdl.NewFileSet(), "devices.vhd", []byte(out))
	if perr != nil {
		t.Fatalf("emitted devices.vhd does not re-parse: %v\n%s", perr, out)
	}
	var ent *vhdl.EntityDecl
	labels := map[string]bool{}
	for _, u := range df.Units {
		switch n := u.(type) {
		case *vhdl.EntityDecl:
			ent = n
		case *vhdl.ArchitectureBody:
			for _, s := range n.Stmts {
				if i, ok := s.(*vhdl.InstantiationStmt); ok {
					labels[i.Label] = true
				}
			}
		}
	}
	if ent == nil {
		t.Fatal("no devices entity emitted")
	}
	got := emittedEntityPortsTyped(ent)
	want := goldenEntityPortsTyped(t, filepath.Join(root, "targets/boards/mimas_v2/devices.vhd"))
	if len(want) == 0 {
		t.Fatal("golden devices.vhd has no entity ports — parse/path problem")
	}
	if !equalStringsDE(got, want) {
		t.Errorf("devices entity ports mismatch (%d vs golden %d):\n got=%v\n want=%v", len(got), len(want), got, want)
	}
	for _, d := range []string{"aic0", "cache_ctrl", "flash", "gpio", "uart0"} {
		if !labels[d] {
			t.Errorf("device %q not instantiated", d)
		}
	}
	for _, te := range []string{"cpus", "ddr_ctrl", "ddr_ram_mux", "fpga_reboot", "pll_250"} {
		if labels[te] {
			t.Errorf("%q must NOT be in devices.vhd", te)
		}
	}
	t.Logf("mimas_v2 devices entity: %d ports (golden %d)", len(got), len(want))
}

func equalStringsDE(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
