package emit

import (
	"sort"

	"github.com/j-core/jcore-soc/tools/socgen/elaborate"
	"github.com/j-core/jcore-soc/tools/socgen/iface"
	"github.com/j-core/jcore-soc/tools/socgen/vhdl"
)

// stdContext is the fixed base context shared by devices.vhd, soc.vhd and
// pad_ring.vhd (faithful to Clojure std-uses), in this order.
func stdContext() []vhdl.Node {
	return []vhdl.Node{
		&vhdl.LibraryClause{Names: []string{"ieee"}},
		&vhdl.UseClause{Names: []string{"ieee.std_logic_1164.all"}},
		&vhdl.UseClause{Names: []string{"ieee.numeric_std.all"}},
		&vhdl.UseClause{Names: []string{"work.config.all"}},
		&vhdl.UseClause{Names: []string{"work.clk_config.all"}},
	}
}

// pkgForMark resolves mark to its declaring work package. A mark in exactly one
// package resolves to it (unchanged). An ambiguous mark (>1 package) is
// disambiguated via the `use` clauses of the bound entity (among owners) that has
// a port of that type; it falls back to TypePackage (last-indexed) when no such
// owner is found.
func pkgForMark(lib *iface.Library, mark string, owners []*iface.Entity) (string, bool) {
	cands := lib.TypePackages(mark)
	switch len(cands) {
	case 0:
		return "", false
	case 1:
		return cands[0], true
	}
	for _, e := range owners {
		if e == nil || !entityHasPortType(e, mark) {
			continue
		}
		uses := map[string]bool{}
		for _, u := range e.Uses {
			uses[lc(u)] = true
		}
		for _, c := range cands {
			if uses[lc(c)] {
				return c, true
			}
		}
	}
	return lib.TypePackage(mark)
}

// entityHasPortType reports whether e has a port whose type mark is mark.
func entityHasPortType(e *iface.Entity, mark string) bool {
	markLC := lc(mark)
	for _, p := range e.Ports {
		if lc(p.Type.Mark) == markLC {
			return true
		}
	}
	return false
}

// packagesScoped resolves marks to distinct sorted packages, disambiguating
// ambiguous marks via owners.
func packagesScoped(lib *iface.Library, marks []string, owners []*iface.Entity) []string {
	if lib == nil {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	for _, m := range marks {
		if pkg, ok := pkgForMark(lib, m, owners); ok && !seen[pkg] {
			seen[pkg] = true
			out = append(out, pkg)
		}
	}
	sort.Strings(out)
	return out
}

// boundEntities returns the bound iface.Entity of each resolved entity (nil-skipped).
func boundEntities(ents map[string]*elaborate.ResolvedEntity) []*iface.Entity {
	out := make([]*iface.Entity, 0, len(ents))
	for _, re := range ents {
		if re.Entity != nil {
			out = append(out, re.Entity)
		}
	}
	return out
}

// mergeSortedPkg returns pkgs with extra inserted if absent, kept sorted+distinct.
func mergeSortedPkg(pkgs []string, extra string) []string {
	for _, p := range pkgs {
		if p == extra {
			return pkgs
		}
	}
	out := append(append([]string{}, pkgs...), extra)
	sort.Strings(out)
	return out
}

// workUses turns sorted package names into `use work.<pkg>.all` clauses.
func workUses(pkgs []string) []vhdl.Node {
	out := make([]vhdl.Node, 0, len(pkgs))
	for _, p := range pkgs {
		out = append(out, &vhdl.UseClause{Names: []string{"work." + p + ".all"}})
	}
	return out
}

// signalMarks returns the type mark of every global signal (nil-safe).
func signalMarks(res *elaborate.Resolution) []string {
	out := make([]string, 0, len(res.Signals))
	for _, s := range res.Signals {
		if s.Type != nil {
			out = append(out, s.Type.Mark)
		}
	}
	return out
}

// deviceContext is the context for devices.vhd: stdContext + data_bus_pack +
// the packages of every device port's type, sorted.
func deviceContext(res *elaborate.Resolution) []vhdl.Node {
	seen := map[string]bool{}
	var pkgs []string
	for _, d := range res.Devices {
		var owners []*iface.Entity
		if rc := res.Classes[lc(d.Class)]; rc != nil && rc.Entity != nil {
			owners = []*iface.Entity{rc.Entity}
		}
		var marks []string
		for _, p := range d.Ports {
			if p.Type != nil {
				marks = append(marks, p.Type.Mark)
			}
		}
		for _, pkg := range packagesScoped(res.Library, marks, owners) {
			if !seen[pkg] {
				seen[pkg] = true
				pkgs = append(pkgs, pkg)
			}
		}
	}
	sort.Strings(pkgs)
	pkgs = mergeSortedPkg(pkgs, "data_bus_pack")
	return append(stdContext(), workUses(pkgs)...)
}

// topContext is the context for soc.vhd: stdContext + the packages of all
// global signals' types, sorted.
func topContext(res *elaborate.Resolution) []vhdl.Node {
	// Global signals are consumed by both top-entities and padring entities; an
	// ambiguous mark may be owned only by a padring entity (e.g. dr_data_*_t lives
	// on ddr_iocells, a padring entity), so both pools must seed disambiguation.
	owners := append(boundEntities(res.TopEntities), boundEntities(res.PadringEntities)...)
	return append(stdContext(), workUses(packagesScoped(res.Library, signalMarks(res), owners))...)
}

// padringContext is topContext plus the unisim library/use, appended after the
// work uses (matching golden pad_ring.vhd).
func padringContext(res *elaborate.Resolution) []vhdl.Node {
	return append(topContext(res),
		&vhdl.LibraryClause{Names: []string{"unisim"}},
		&vhdl.UseClause{Names: []string{"unisim.vcomponents.all"}},
	)
}
