package emit

import (
	"github.com/j-core/jcore-soc/tools/socgen/elaborate"
	"github.com/j-core/jcore-soc/tools/socgen/vhdl"
)

// irqDecls renders the IRQ model's signal declarations: irqs<cpu> (8-bit, zeroed)
// and the OR-source scalars.
func irqDecls(m *elaborate.IRQModel) []vhdl.Decl {
	if m == nil {
		return nil
	}
	out := make([]vhdl.Decl, 0, len(m.Signals))
	for _, s := range m.Signals {
		if s.Width == 8 {
			out = append(out, &vhdl.SignalDecl{
				Names:       []string{s.Name},
				SubtypeMark: "std_logic_vector",
				Constraint:  &vhdl.ParenExpr{X: &vhdl.Range{Left: intLit(7), Dir: vhdl.DOWNTO, Right: intLit(0)}},
				Default:     othersZero(),
			})
		} else {
			out = append(out, &vhdl.SignalDecl{Names: []string{s.Name}, SubtypeMark: stdLogicMark})
		}
	}
	return out
}

// irqAssigns renders the OR-combine concurrent assignments.
func irqAssigns(m *elaborate.IRQModel) []vhdl.Stmt {
	if m == nil {
		return nil
	}
	out := make([]vhdl.Stmt, 0, len(m.OrAssigns))
	for _, a := range m.OrAssigns {
		out = append(out, concAssign(&vhdl.Ident{Name: a.Target}, orExpr(a.Sources)))
	}
	return out
}

// orExpr folds `a or b or …` over the source idents.
func orExpr(sources []string) vhdl.Expr {
	if len(sources) == 0 {
		return &vhdl.Ident{Name: "'0'"}
	}
	e := vhdl.Expr(&vhdl.Ident{Name: sources[0]})
	for _, s := range sources[1:] {
		e = &vhdl.BinaryExpr{X: e, Op: vhdl.OR, Y: &vhdl.Ident{Name: s}}
	}
	return e
}

// vectorNumbersAgg renders an 8-entry positional aggregate of x"NN" hex literals.
func vectorNumbersAgg(vn [8]int) vhdl.Expr {
	agg := &vhdl.Aggregate{}
	for _, v := range vn {
		agg.Elems = append(agg.Elems, &vhdl.ElementAssoc{X: &vhdl.BasicLit{Kind: vhdl.BITSTRINGLIT, Value: hex8(v)}})
	}
	return agg
}

// hex8 renders an 8-bit hex bit-string literal: x"NN".
func hex8(v int) string {
	const digits = "0123456789ABCDEF"
	return `x"` + string([]byte{digits[(v>>4)&0xF], digits[v&0xF]}) + `"`
}
