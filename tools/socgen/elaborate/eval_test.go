package elaborate

import (
	"testing"

	"github.com/j-core/jcore-soc/tools/socgen/vhdl"
)

// parseConstraint parses a VHDL constrained subtype from an interface decl and returns
// its SubtypeMark + Constraint expr, e.g. parseConstraint(t, "std_logic_vector(NUM_CS-1 downto 0)").
func parseConstraint(t *testing.T, typ string) (string, vhdl.Expr) {
	t.Helper()
	src := "entity e is port (p : in " + typ + "); end entity;"
	df, err := vhdl.ParseFile(vhdl.NewFileSet(), "t.vhd", []byte(src))
	if err != nil {
		t.Fatalf("parse %q: %v", typ, err)
	}
	ent := df.Units[0].(*vhdl.EntityDecl)
	id := ent.Ports[0]
	return id.SubtypeMark, id.Constraint
}

func TestEvalInt(t *testing.T) {
	env := map[string]int64{"num_cs": 2, "n": 4}
	cases := []struct {
		typ  string
		want string // ResolvedType.String()
	}{
		{"std_logic_vector(num_cs-1 downto 0)", "std_logic_vector(1 downto 0)"},
		{"std_logic_vector(8*n-1 downto 0)", "std_logic_vector(31 downto 0)"},
		{"std_logic_vector(n+1 downto 0)", "std_logic_vector(5 downto 0)"},
		{"std_logic_vector(0 to n)", "std_logic_vector(0 to 4)"},
		{"std_logic", "std_logic"},
		{"std_logic_vector(missing-1 downto 0)", "std_logic_vector(missing - 1 downto 0)"},        // symbolic: missing not in env
		{"std_logic_vector(num_cs-1 downto missing)", "std_logic_vector(num_cs - 1 downto missing)"}, // half-symbolic: one concrete, one missing -> stays symbolic
	}
	for _, c := range cases {
		mark, con := parseConstraint(t, c.typ)
		got := resolveType(mark, con, env).String()
		if got != c.want {
			t.Errorf("resolveType(%q) = %q, want %q", c.typ, got, c.want)
		}
	}
}
