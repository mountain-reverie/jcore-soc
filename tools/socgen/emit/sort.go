package emit

import (
	"sort"

	"github.com/j-core/jcore-soc/tools/socgen/vhdl"
)

// sortInstMaps sorts every instantiation's generic-map and port-map associations
// alphabetically by formal, matching the vmagic golden. Done in the emitted AST
// (not the printer) so the printer's order-preserving corpus round-trip is
// unaffected.
func sortInstMaps(df *vhdl.DesignFile) {
	vhdl.Inspect(df, func(n vhdl.Node) bool {
		if inst, ok := n.(*vhdl.InstantiationStmt); ok {
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
