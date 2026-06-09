package emit

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/j-core/jcore-soc/tools/socgen/board"
	"github.com/j-core/jcore-soc/tools/socgen/elaborate"
	"github.com/j-core/jcore-soc/tools/socgen/vhdl"
)

// goldenPinPorts returns the golden pad_ring entity's `pin_`-prefixed ports as
// sorted "name:dir".
func goldenPinPorts(t *testing.T, path string) []string {
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
					if strings.HasPrefix(n, "pin_") {
						out = append(out, n+":"+d)
					}
				}
			}
		}
	}
	sort.Strings(out)
	return out
}

func TestPadRingParityMimasV2(t *testing.T) {
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
	out, derr := PadRing(res)
	if derr != nil {
		t.Logf("emit notes: %v", derr)
	}
	df, perr := vhdl.ParseFile(vhdl.NewFileSet(), "pad_ring.vhd", []byte(out))
	if perr != nil {
		t.Fatalf("emitted pad_ring.vhd does not re-parse: %v\n%s", perr, out)
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
		t.Fatal("no pad_ring entity")
	}

	// pin ports (name:dir).
	var got []string
	gotSet := map[string]bool{}
	for _, p := range ent.Ports {
		d := p.Mode
		if d == "" {
			d = "in"
		}
		for _, n := range p.Names {
			if strings.HasPrefix(n, "pin_") {
				got = append(got, n+":"+d)
				gotSet[n+":"+d] = true
			}
		}
	}
	sort.Strings(got)
	want := goldenPinPorts(t, filepath.Join(root, "targets/boards/mimas_v2/pad_ring.vhd"))
	if len(want) == 0 {
		t.Fatal("golden pad_ring has no pin_* ports — parse/path problem")
	}

	// FRAME PARITY (the P5d-a assertion): every golden pin_* port must be
	// emitted with the matching direction. This is the strong, load-bearing
	// check — it proves padDir + port emission for all 71 real pads (LEDs,
	// DRAM, SD, SPI, UART, seven-segment, clk) are correct.
	var missing []string
	for _, w := range want {
		if !gotSet[w] {
			missing = append(missing, w)
		}
	}
	if len(missing) != 0 {
		t.Errorf("pad_ring is MISSING golden pin ports (or wrong direction): %v\n got=%v", missing, got)
	}

	// EXACT PARITY: with the missing-pin drop (io_p* pads, whose io_p<n> signals
	// no device declares, are dropped — faithful to Clojure :missing), the
	// emitted pin_* port set must EXACTLY equal the golden's. No extras.
	if !equalStringsDE(got, want) {
		t.Errorf("pad_ring pin ports != golden:\n got=%v\n want=%v", got, want)
	}

	// a couple of known LOC attributes present (normalized).
	norm := func(s string) string { return strings.ReplaceAll(strings.ToLower(s), " ", "") }
	for _, want := range []string{`attribute loc of pin_led0 : signal is "t18"`} {
		if !strings.Contains(norm(out), norm(want)) {
			t.Errorf("missing pad attribute %q", want)
		}
	}

	// soc instance + the 5 padring-entity instances (right kinds).
	if s := insts["soc"]; s == nil || s.Unit != "work.soc" || s.Arch != "impl" {
		t.Errorf("soc instance missing/wrong: %+v", s)
	}
	for _, pe := range []string{"pll_250", "ddr_clkgen", "ddr_iocells", "reset_gen", "spi_merge"} {
		if insts[pe] == nil {
			t.Errorf("padring-entity %q not instantiated", pe)
		}
	}
	if di := insts["ddr_iocells"]; di != nil && di.UnitKind != vhdl.CONFIGURATION {
		t.Errorf("ddr_iocells should be a configuration instance: %+v", di)
	}
	t.Logf("mimas_v2 pad_ring: %d pin ports (golden %d, exact parity, no io_p); insts=%d",
		len(got), len(want), len(insts))
}
