package emit

import (
	"github.com/j-core/jcore-soc/tools/socgen/iface"
	"github.com/j-core/jcore-soc/tools/socgen/vhdl"
)

// zeroVal builds the VHDL zero/default value for a type (faithful to vmagic zero-val):
// a record -> aggregate of per-field zeros; an enum -> its leftmost literal; an
// array/vector -> (others => '0'); a scalar bit type -> '0'; a subtype -> its base
// type's zero. Unknown/unresolvable -> (others => '0').
func zeroVal(mark string, lib *iface.Library) vhdl.Expr {
	if lib != nil {
		if te, ok := lib.ResolveType(mark); ok {
			switch n := te.Node.(type) {
			case *vhdl.TypeDecl:
				switch def := n.Def.(type) {
				case *vhdl.RecordDef:
					elems := make([]*vhdl.ElementAssoc, 0, len(def.Fields))
					for _, f := range def.Fields {
						for _, fn := range f.Names {
							elems = append(elems, &vhdl.ElementAssoc{
								Choices: []vhdl.Expr{&vhdl.Ident{Name: fn}},
								X:       zeroVal(f.SubtypeMark, lib),
							})
						}
					}
					return &vhdl.Aggregate{Elems: elems}
				case *vhdl.EnumDef:
					if len(def.Lits) > 0 {
						return &vhdl.Ident{Name: def.Lits[0]}
					}
				case *vhdl.ArrayDef:
					return othersZero()
				}
			case *vhdl.SubtypeDecl:
				return zeroVal(n.SubtypeMark, lib) // alias -> recurse on the base type
			}
		}
	}
	switch lc(mark) {
	case "std_logic", "std_ulogic", "bit":
		return &vhdl.BasicLit{Kind: vhdl.CHARLIT, Value: "'0'"}
	}
	return othersZero()
}

func othersZero() vhdl.Expr {
	return &vhdl.Aggregate{Elems: []*vhdl.ElementAssoc{{
		Choices: []vhdl.Expr{&vhdl.Ident{Name: "others"}},
		X:       &vhdl.BasicLit{Kind: vhdl.CHARLIT, Value: "'0'"},
	}}}
}
