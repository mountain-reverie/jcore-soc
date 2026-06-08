package emit

import (
	"strings"
	"testing"

	"github.com/j-core/jcore-soc/tools/socgen/iface"
	"github.com/j-core/jcore-soc/tools/socgen/vhdl"
)

func libFrom(t *testing.T, src string) *iface.Library {
	t.Helper()
	df, perr := vhdl.ParseFile(vhdl.NewFileSet(), "p.vhd", []byte(src))
	if perr != nil {
		t.Fatalf("parse: %v", perr)
	}
	lib, err := iface.Extract([]*vhdl.DesignFile{df})
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	return lib
}

// renderExpr prints a single expr by placing it as a signal default and printing.
func renderExpr(t *testing.T, e vhdl.Expr) string {
	t.Helper()
	df := &vhdl.DesignFile{Units: []vhdl.DesignUnit{
		&vhdl.ArchitectureBody{Name: "a", Entity: "e", Decls: []vhdl.Decl{
			&vhdl.SignalDecl{Names: []string{"s"}, SubtypeMark: "integer", Default: e},
		}},
	}}
	return vhdl.Print(df)
}

func TestZeroVal(t *testing.T) {
	src := `package p is
  type cmd_t is (INTERRUPT, RESET, BREAK);
  type byte_arr_t is array (0 to 7) of std_logic;
  subtype nib_t is cmd_t;
  type ev_t is record
    en : std_logic;
    cmd : cmd_t;
    vec : std_logic_vector(7 downto 0);
  end record;
end package;`
	lib := libFrom(t, src)

	// ArrayDef branch (a library-resolved array type, not the unknown fallback).
	if g := renderExpr(t, zeroVal("byte_arr_t", lib)); !strings.Contains(g, "(others => '0')") {
		t.Errorf("array zero = %q", g)
	}
	// SubtypeDecl branch recurses to its base enum.
	if g := renderExpr(t, zeroVal("nib_t", lib)); !strings.Contains(g, "INTERRUPT") {
		t.Errorf("subtype zero = %q", g)
	}

	if g := renderExpr(t, zeroVal("std_logic", lib)); !strings.Contains(g, "'0'") {
		t.Errorf("std_logic zero = %q", g)
	}
	if g := renderExpr(t, zeroVal("std_logic_vector", lib)); !strings.Contains(g, "(others => '0')") {
		t.Errorf("vector zero = %q", g)
	}
	if g := renderExpr(t, zeroVal("cmd_t", lib)); !strings.Contains(g, "INTERRUPT") {
		t.Errorf("enum zero = %q", g)
	}
	g := renderExpr(t, zeroVal("ev_t", lib))
	for _, want := range []string{"en => '0'", "cmd => INTERRUPT", "vec => (others => '0')"} {
		if !strings.Contains(g, want) {
			t.Errorf("record zero missing %q: %s", want, g)
		}
	}
}
