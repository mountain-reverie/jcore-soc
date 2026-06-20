package emit

import (
	"strings"
	"testing"

	"github.com/j-core/jcore-soc/tools/socgen/design"
	"github.com/j-core/jcore-soc/tools/socgen/vhdl"
)

func vectorNumbersCount(inst *vhdl.InstantiationStmt) int {
	n := 0
	for _, g := range inst.GenericMap {
		if g.Formal == "vector_numbers" {
			n++
		}
	}
	return n
}

// An explicit vector_numbers generic overrides the IRQ-network-derived vecAgg:
// it is emitted exactly once (the explicit value), not duplicated alongside
// vecAgg (which would be a "generic already associated" VHDL error). Needed for
// boards whose IRQ source is a raw pin, not an irq-declaring device.
func TestInstStmtExplicitVectorNumbersOverridesVecAgg(t *testing.T) {
	generics := map[string]design.Value{"vector_numbers": {Kind: design.KindExpr, Text: "MY_VECTORS"}}
	vecAgg := &vhdl.Ident{Name: "AUTO_AGG"}
	inst := instStmt("aic0", "aic", "", nil, generics, nil, nil, "", nil, nil, vecAgg)
	if c := vectorNumbersCount(inst); c != 1 {
		t.Fatalf("vector_numbers emitted %d times, want 1 (explicit must override vecAgg)", c)
	}
	out := printStmts([]vhdl.Stmt{inst})
	if !strings.Contains(out, "MY_VECTORS") {
		t.Errorf("explicit vector_numbers value missing in:\n%s", out)
	}
	if strings.Contains(out, "AUTO_AGG") {
		t.Errorf("auto vecAgg must be suppressed when explicit vector_numbers is set:\n%s", out)
	}
}

// Control: with no explicit vector_numbers generic, the auto vecAgg is emitted.
func TestInstStmtVecAggEmittedWhenNoExplicit(t *testing.T) {
	inst := instStmt("aic0", "aic", "", nil, nil, nil, nil, "", nil, nil, &vhdl.Ident{Name: "AUTO_AGG"})
	if c := vectorNumbersCount(inst); c != 1 {
		t.Fatalf("vecAgg vector_numbers emitted %d times, want 1", c)
	}
	out := printStmts([]vhdl.Stmt{inst})
	if !strings.Contains(out, "AUTO_AGG") {
		t.Errorf("vecAgg should be emitted when no explicit vector_numbers:\n%s", out)
	}
}
