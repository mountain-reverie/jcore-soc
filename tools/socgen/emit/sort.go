package emit

import (
	"sort"

	"github.com/j-core/jcore-soc/tools/socgen/vhdl"
)

// sortInstMaps sorts every NAMED (entity/component/configuration) instantiation's
// generic-map and port-map associations alphabetically by formal, matching the
// vmagic golden. Bare-component instantiations (UnitKind == 0) — the Xilinx
// primitive buffers (IOBUF/IBUFDS/OBUFDS) — are exempt: their maps are built in
// the primitive's declared port order (e.g. I, T, O, IO), which the golden
// preserves. Done in the emitted AST (not the printer) so the printer's
// order-preserving corpus round-trip is unaffected.
func sortInstMaps(df *vhdl.DesignFile) {
	vhdl.Inspect(df, func(n vhdl.Node) bool {
		if inst, ok := n.(*vhdl.InstantiationStmt); ok && inst.UnitKind != 0 {
			sortAssoc(inst.GenericMap)
			sortAssoc(inst.PortMap)
		}
		return true
	})
}

// sortAssoc stable-sorts elems by Formal in place, unless any element is
// positional (Formal == ""), in which case order is preserved (positional
// association order is significant).
func sortAssoc(elems []*vhdl.AssocElement) {
	for _, e := range elems {
		if e.Formal == "" {
			return
		}
	}
	sort.SliceStable(elems, func(i, j int) bool { return elems[i].Formal < elems[j].Formal })
}
