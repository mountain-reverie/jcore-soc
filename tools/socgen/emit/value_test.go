package emit

import (
	"strings"
	"testing"

	"github.com/j-core/jcore-soc/tools/socgen/design"
	"github.com/j-core/jcore-soc/tools/socgen/vhdl"
)

// render prints a single expr by placing it as a signal default in a throwaway
// architecture and rendering the design file.
func render(t *testing.T, e vhdl.Expr) string {
	t.Helper()
	df := &vhdl.DesignFile{Units: []vhdl.DesignUnit{
		&vhdl.ArchitectureBody{Name: "arch_test", Entity: "entity_test", Decls: []vhdl.Decl{
			&vhdl.SignalDecl{Names: []string{"s"}, SubtypeMark: "integer", Default: e},
		}},
	}}
	return vhdl.Print(df)
}

func TestEmitValueBoolUppercase(t *testing.T) {
	tr := emitValue(design.Value{Kind: design.KindBool, Bool: true})
	if id, ok := tr.(*vhdl.Ident); !ok || id.Name != "TRUE" {
		t.Errorf("emitValue(true) = %#v, want Ident TRUE", tr)
	}
	fl := emitValue(design.Value{Kind: design.KindBool, Bool: false})
	if id, ok := fl.(*vhdl.Ident); !ok || id.Name != "FALSE" {
		t.Errorf("emitValue(false) = %#v, want Ident FALSE", fl)
	}
}

func TestEmitValue(t *testing.T) {
	cases := []struct {
		name string
		v    design.Value
		want string
	}{
		{"expr", design.Value{Kind: design.KindExpr, Text: "num_cs-1"}, "num_cs-1"},
		{"int", design.Value{Kind: design.KindInt, Int: 8}, "8"},
		{"bool", design.Value{Kind: design.KindBool, Bool: true}, "TRUE"},
		{"str", design.Value{Kind: design.KindStr, Text: "hello"}, `"hello"`},
		{"float_whole", design.Value{Kind: design.KindFloat, Float: 1.0}, "1.0"},
		{"float_frac", design.Value{Kind: design.KindFloat, Float: 1.5}, "1.5"},
		{"float_exp", design.Value{Kind: design.KindFloat, Float: 1e21}, "1.0e+21"},
		{"map", design.Value{Kind: design.KindMap}, "open"},
		{"str_quoted", design.Value{Kind: design.KindStr, Text: `a"b`}, `"a""b"`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := render(t, emitValue(c.v))
			if !strings.Contains(got, c.want) {
				t.Errorf("emitValue(%+v) rendered %q, want substring %q", c.v, got, c.want)
			}
		})
	}
}
