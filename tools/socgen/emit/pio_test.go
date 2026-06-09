package emit

import (
	"strings"
	"testing"

	"github.com/j-core/jcore-soc/tools/socgen/elaborate"
	"github.com/j-core/jcore-soc/tools/socgen/vhdl"
)

func TestPioStatements(t *testing.T) {
	c0, c1 := 0, 1
	res := &elaborate.Resolution{Pio: []elaborate.PioBit{
		{Idx: 0},              // loopback
		{Idx: 1},              // loopback
		{Idx: 19, Const: &c0}, // const '0'
		{Idx: 20, Const: &c1}, // const '1' (verifies the value is not hardcoded)
	}}
	stmts := pioStatements(res)
	if len(stmts) != 4 {
		t.Fatalf("want 4 stmts, got %d", len(stmts))
	}
	out := renderPioStmts(t, stmts)
	for _, w := range []string{"pi(0) <= po(0)", "pi(1) <= po(1)", "pi(19) <= '0'", "pi(20) <= '1'"} {
		if !strings.Contains(out, w) {
			t.Errorf("pioStatements missing %q:\n%s", w, out)
		}
	}
}

// renderPioStmts prints stmts inside a throwaway architecture.
func renderPioStmts(t *testing.T, stmts []vhdl.Stmt) string {
	t.Helper()
	df := &vhdl.DesignFile{Units: []vhdl.DesignUnit{
		&vhdl.ArchitectureBody{Name: "i", Entity: "e", Stmts: stmts},
	}}
	return vhdl.Print(df)
}
