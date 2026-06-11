package elaborate

import (
	"sort"

	"github.com/j-core/jcore-soc/tools/socgen/design"
)

// resolvePio expands the design's system.pio entries into per-bit loopback/const
// assignments, sorted by bit index. Faithful to generate.clj pio-stmts (the
// in/out/pin sub-cases are mimas-unused and deferred). Nil-safe.
//
// Entries are assumed disjoint (as the spec defines them per-bit/per-range):
// overlapping ranges would emit two assignments for the same pi(idx), i.e. a
// VHDL multiple-driver — the design spec is responsible for disjointness.
func resolvePio(d *design.Design) []PioBit {
	if d == nil || d.System == nil {
		return nil
	}
	var bits []PioBit
	for _, e := range d.System.Pio {
		for idx := e.Lo; idx <= e.Hi; idx++ {
			b := PioBit{Idx: idx, Name: e.Name}
			if e.Const != nil {
				c := *e.Const
				b.Const = &c
			}
			bits = append(bits, b)
		}
	}
	sort.Slice(bits, func(i, j int) bool { return bits[i].Idx < bits[j].Idx })
	return bits
}
