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

// packagesOf resolves each type mark to its declaring work package, returning
// the distinct package names sorted. Marks not in a work package (std types,
// unsigned/signed) are skipped, as is a nil library.
func packagesOf(lib *iface.Library, marks []string) []string {
	if lib == nil {
		return nil
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(marks))
	for _, m := range marks {
		if pkg, ok := lib.TypePackage(m); ok && !seen[pkg] {
			seen[pkg] = true
			out = append(out, pkg)
		}
	}
	sort.Strings(out)
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
	var marks []string
	for _, d := range res.Devices {
		for _, p := range d.Ports {
			if p.Type != nil {
				marks = append(marks, p.Type.Mark)
			}
		}
	}
	pkgs := mergeSortedPkg(packagesOf(res.Library, marks), "data_bus_pack")
	return append(stdContext(), workUses(pkgs)...)
}

// topContext is the context for soc.vhd: stdContext + the packages of all
// global signals' types, sorted.
func topContext(res *elaborate.Resolution) []vhdl.Node {
	return append(stdContext(), workUses(packagesOf(res.Library, signalMarks(res)))...)
}

// padringContext is topContext plus the unisim library/use, appended after the
// work uses (matching golden pad_ring.vhd).
func padringContext(res *elaborate.Resolution) []vhdl.Node {
	return append(topContext(res),
		&vhdl.LibraryClause{Names: []string{"unisim"}},
		&vhdl.UseClause{Names: []string{"unisim.vcomponents.all"}},
	)
}
