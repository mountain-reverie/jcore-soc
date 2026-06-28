package elaborate

import (
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/j-core/jcore-soc/tools/socgen/board"
	"github.com/j-core/jcore-soc/tools/socgen/vhdl"
)

func entityPortSet(t *testing.T, path string) []string {
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
					out = append(out, n+":"+d)
				}
			}
		}
	}
	sort.Strings(out)
	return out
}

func locSet(ps []PortLoc) []string {
	out := make([]string, 0, len(ps))
	for _, p := range ps {
		out = append(out, p.Name+":"+p.Dir)
	}
	sort.Strings(out)
	return out
}

func eqSet(a, b []string) bool {
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

func TestEntityNamingMimasV2(t *testing.T) {
	root := os.Getenv("JCORE_SOC_ROOT")
	if root == "" {
		t.Skip("JCORE_SOC_ROOT not set")
	}
	b, lerr := board.Load(root, "mimas_v2", "")
	if b == nil || b.Design == nil {
		t.Fatalf("board.Load: %v", lerr)
	}
	res, rerr := Elaborate(b)
	if rerr != nil {
		t.Logf("elaborate notes: %v", rerr)
	}

	// Unification fact: cpu0_event_o has a top driver + device consumer.
	if s := res.Signals["cpu0_event_o"]; s != nil {
		var top, dev bool
		for _, p := range s.Ports {
			if p.Context.Kind == "top" && p.Dir == dirOut {
				top = true
			}
			if p.Context.Kind == "device" {
				dev = true
			}
		}
		if !top || !dev {
			t.Errorf("cpu0_event_o not unified: top=%v dev=%v", top, dev)
		}
	} else {
		t.Error("cpu0_event_o missing from net-list")
	}

	// Orphan top-entity-prefixed signals are gone.
	orphans := 0
	for n := range res.Signals {
		if hasPrefix2(n, "cpus_") || hasPrefix2(n, "ddr_ram_mux_") {
			orphans++
		}
	}
	if orphans > 0 {
		t.Errorf("expected 0 orphan top-entity-prefixed signals, got %d", orphans)
	}

	// Headline: categorization matches the golden entity ports (27 / 16).
	if res.SignalLocations == nil {
		t.Fatal("SignalLocations nil")
	}
	wantDev := entityPortSet(t, filepath.Join(root, "targets/boards/mimas_v2/devices.vhd"))
	gotDev := locSet(res.SignalLocations.TopDevices)
	if !eqSet(gotDev, wantDev) {
		t.Errorf("devices ports mismatch (%d vs golden %d):\n got=%v\n want=%v", len(gotDev), len(wantDev), gotDev, wantDev)
	}
	wantSoc := entityPortSet(t, filepath.Join(root, "targets/boards/mimas_v2/soc.vhd"))
	gotSoc := locSet(res.SignalLocations.PadringTop)
	if !eqSet(gotSoc, wantSoc) {
		t.Errorf("soc ports mismatch (%d vs golden %d):\n got=%v\n want=%v", len(gotSoc), len(wantSoc), gotSoc, wantSoc)
	}
	t.Logf("mimas_v2: TopDevices=%d (golden %d), PadringTop=%d (golden %d), orphans=%d", len(gotDev), len(wantDev), len(gotSoc), len(wantSoc), orphans)
}

func hasPrefix2(s, p string) bool { return len(s) >= len(p) && s[:len(p)] == p }
