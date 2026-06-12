// Package emit renders the elaborated SoC model (P4 output) to VHDL/other
// artifacts by building tools/socgen/vhdl AST nodes and printing them.
package emit

import (
	"sort"
	"strconv"
	"strings"

	"github.com/j-core/jcore-soc/tools/socgen/design"
	"github.com/j-core/jcore-soc/tools/socgen/elaborate"
	"github.com/j-core/jcore-soc/tools/socgen/vhdl"
)

// lc lower-cases and trims, matching the elaborate package's key/label convention.
func lc(s string) string { return strings.ToLower(strings.TrimSpace(s)) }

// vhdlEscape escapes a string for use inside a VHDL string literal: an embedded
// double-quote is doubled per VHDL-93 lexical rules.
func vhdlEscape(s string) string { return strings.ReplaceAll(s, `"`, `""`) }

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
		return &vhdl.BasicLit{Kind: vhdl.REAL, Value: vhdlReal(v.Float)}
	case design.KindBool:
		if v.Bool {
			return &vhdl.Ident{Name: "TRUE"}
		}
		return &vhdl.Ident{Name: "FALSE"}
	case design.KindStr:
		return &vhdl.BasicLit{Kind: vhdl.STRINGLIT, Value: `"` + vhdlEscape(v.Text) + `"`}
	case design.KindMap:
		return &vhdl.Ident{Name: "open"}
	default: // KindExpr — verbatim VHDL text
		return &vhdl.Ident{Name: v.Text}
	}
}

// numVal renders an integer constant on a formal of resolved type t, faithful to
// vmagic num-val: a std_logic scalar 0/1 → '0'/'1'; an array type (std_logic_vector
// etc.) with concrete width → a sized hex/binary literal (numLiteral); anything else
// falls back to emitValue (decimal integers, strings, exprs).
func numVal(t *elaborate.ResolvedType, v design.Value) vhdl.Expr {
	if v.Kind == design.KindInt && t != nil {
		if t.Mark == stdLogicMark && (v.Int == 0 || v.Int == 1) {
			val := "'0'"
			if v.Int == 1 {
				val = "'1'"
			}
			return &vhdl.BasicLit{Kind: vhdl.CHARLIT, Value: val}
		}
		if vectorMark(t.Mark) && t.Left != nil && t.Right != nil {
			w := *t.Left - *t.Right
			if w < 0 {
				w = -w
			}
			return numLiteral(w+1, v.Int)
		}
	}
	return emitValue(v)
}

// numLiteral renders val into a width-bit literal (vmagic num-literal): a hex
// bit-string when width is a multiple of 4, otherwise a binary string literal;
// both zero-padded to the type width.
func numLiteral(width int, val int64) vhdl.Expr {
	if width%4 == 0 {
		return &vhdl.BasicLit{Kind: vhdl.STRINGLIT, Value: `x"` + zeroPad(strconv.FormatInt(val, 16), width/4) + `"`}
	}
	return &vhdl.BasicLit{Kind: vhdl.STRINGLIT, Value: `"` + zeroPad(strconv.FormatInt(val, 2), width) + `"`}
}

// zeroPad left-pads s with '0' to width characters, truncating from the left
// when len(s) > width (matching vmagic zero-pad). s must be a non-negative base-2
// or base-16 digit string; a leading '-' would be silently truncated.
func zeroPad(s string, width int) string {
	if len(s) >= width {
		return s[len(s)-width:]
	}
	return strings.Repeat("0", width-len(s)) + s
}

// vectorMark reports whether a type mark is an array/vector type whose integer
// constants render as a sized bit-string (vs an integer scalar → decimal).
func vectorMark(mark string) bool {
	switch lc(mark) {
	case "std_logic_vector", "std_ulogic_vector", "unsigned", "signed", "bit_vector":
		return true
	}
	return false
}

// vhdlReal formats a float as a VHDL-93 real literal. A VHDL real literal
// requires a decimal point in the mantissa, even in exponential form (e.g.
// "1.0e21", not "1e21"). strconv's shortest 'g' form omits it, so we insert
// ".0" into the mantissa when absent.
func vhdlReal(f float64) string {
	s := strconv.FormatFloat(f, 'g', -1, 64)
	mantissa, exp := s, ""
	if i := strings.IndexAny(s, "eE"); i >= 0 {
		mantissa, exp = s[:i], s[i:]
	}
	if !strings.Contains(mantissa, ".") {
		mantissa += ".0"
	}
	return mantissa + exp
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
