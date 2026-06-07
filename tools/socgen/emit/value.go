// Package emit renders the elaborated SoC model (P4 output) to VHDL/other
// artifacts by building tools/socgen/vhdl AST nodes and printing them.
package emit

import (
	"sort"
	"strconv"
	"strings"

	"github.com/j-core/jcore-soc/tools/socgen/design"
	"github.com/j-core/jcore-soc/tools/socgen/vhdl"
)

// lc lower-cases and trims, matching the elaborate package's key/label convention.
func lc(s string) string { return strings.ToLower(strings.TrimSpace(s)) }

// intLit builds an integer literal expression.
func intLit(i int) vhdl.Expr { return &vhdl.BasicLit{Kind: vhdl.INT, Value: strconv.Itoa(i)} }

// emitValue renders a design.Value as a VHDL expression. KindExpr is verbatim VHDL
// text (printed as-is via an Ident); typed scalars become literals. A KindMap is
// not expected as a generic/port value and degrades to `open`.
func emitValue(v design.Value) vhdl.Expr {
	switch v.Kind {
	case design.KindInt:
		return &vhdl.BasicLit{Kind: vhdl.INT, Value: strconv.FormatInt(v.Int, 10)}
	case design.KindFloat:
		s := strconv.FormatFloat(v.Float, 'g', -1, 64)
		if !strings.ContainsAny(s, ".eE") {
			s += ".0"
		}
		return &vhdl.BasicLit{Kind: vhdl.REAL, Value: s}
	case design.KindBool:
		if v.Bool {
			return &vhdl.Ident{Name: "true"}
		}
		return &vhdl.Ident{Name: "false"}
	case design.KindStr:
		escaped := strings.ReplaceAll(v.Text, `"`, `""`)
		return &vhdl.BasicLit{Kind: vhdl.STRINGLIT, Value: `"` + escaped + `"`}
	case design.KindMap:
		return &vhdl.Ident{Name: "open"}
	default: // KindExpr — verbatim VHDL text
		return &vhdl.Ident{Name: v.Text}
	}
}

// sortedKeys returns the map keys of a generic map in deterministic order.
func sortedKeys(m map[string]design.Value) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}
