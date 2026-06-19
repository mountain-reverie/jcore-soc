package emit

import (
	"errors"
	"sort"
	"strconv"
	"strings"

	"github.com/j-core/jcore-soc/tools/socgen/elaborate"
	"github.com/j-core/jcore-soc/tools/socgen/vhdl"
)

// SoC renders soc.vhd: a `soc` entity (PadringTop ports) and an architecture that
// declares the Top/TopExtra/zero-out signals and instantiates the top entities +
// the devices architecture, zeroing unused signals. Best-effort; never panics.
func SoC(res *elaborate.Resolution) (string, error) {
	if res == nil {
		return "", nil
	}
	var errs []error
	sl := res.SignalLocations

	// declarations: Top internal + TopExtra aliases + zero-out signals
	var declNames []string
	subst := map[string]string{}
	if sl != nil {
		declNames = append(declNames, sl.Top...)
		for name, alias := range sl.TopExtra {
			declNames = append(declNames, alias)
			subst[name] = alias
		}
	}
	zeroNames := zeroOutSignals(res)
	declNames = append(declNames, zeroNames...)
	sort.Strings(declNames)
	decls := make([]vhdl.Decl, 0, len(declNames))
	for _, n := range declNames {
		mark, con := typeToSubtype(typeForDecl(res, n, subst)) // shared with devices.go
		decls = append(decls, &vhdl.SignalDecl{Names: []string{n}, SubtypeMark: mark, Constraint: con})
	}

	// statements: TopExtra assigns, top instantiations, devices instance, zero-out
	stmts := make([]vhdl.Stmt, 0, len(subst)+len(res.TopEntities)+1+len(zeroNames)+1)
	for _, name := range sortedKeysStr(subst) {
		stmts = append(stmts, concAssign(&vhdl.Ident{Name: name}, &vhdl.Ident{Name: subst[name]}))
	}
	for _, name := range sortedTopNames(res.TopEntities) {
		re := res.TopEntities[name]
		if re.Entity == nil && re.Config == nil {
			errs = append(errs, &EmitError{Kind: ErrUnboundEntity, Inst: re.Name})
			continue
		}
		stmts = append(stmts, topInstStmt(re, res))
	}
	stmts = append(stmts, devicesInstStmt(res))
	if len(zeroNames) > 0 {
		stmts = append(stmts, &vhdl.Comment{Text: "Zero out unused signals"})
	}
	for _, n := range zeroNames {
		var mark string
		if s := res.Signals[n]; s != nil && s.Type != nil {
			mark = s.Type.Mark
		}
		stmts = append(stmts, concAssign(&vhdl.Ident{Name: n}, zeroVal(mark, res.Library)))
	}

	df := &vhdl.DesignFile{
		Context: topContext(res),
		Units: []vhdl.DesignUnit{
			&vhdl.EntityDecl{Name: "soc", Ports: socEntityPorts(res)},
			&vhdl.ArchitectureBody{Name: "impl", Entity: "soc", Decls: decls, Stmts: stmts},
		},
	}
	sortInstMaps(df)
	return withBanner(vhdl.Print(df)), errors.Join(errs...)
}

// zeroOutSignals returns the names of zero-context global signals, sorted
// alphabetically for deterministic output.
//
// DIVERGENCE (order): golden emits the zero-out concurrent assignments in the
// Clojure global-signals map-insertion order (not alphabetical). Concurrent-
// statement order is semantically irrelevant in VHDL (reordering is bit-identical
// in simulation and synthesis), so we keep a stable alphabetical order. mimas's
// zero-signals are alphabetical in golden (so it matches); microboard's are not (a
// cosmetic, non-byte-exact difference — moot, as microboard can't be whole-file
// byte-exact anyway: eth_clk_bufs, MB-4). A faithful insertion-order match is
// deferred until turtle_1v0 provides a second example to derive the rule. See MB-6.
func zeroOutSignals(res *elaborate.Resolution) []string {
	var out []string
	for name, s := range res.Signals {
		for _, p := range s.Ports {
			if p.Context.Kind == "zero" {
				out = append(out, name)
				break
			}
		}
	}
	sort.Strings(out)
	return out
}

func sortedKeysStr(m map[string]string) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}

func sortedTopNames(m map[string]*elaborate.ResolvedEntity) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}

// socEntityPorts builds the soc entity's ports from the PadringTop boundary signals.
func socEntityPorts(res *elaborate.Resolution) []*vhdl.InterfaceDecl {
	if res.SignalLocations == nil {
		return nil
	}
	out := make([]*vhdl.InterfaceDecl, 0, len(res.SignalLocations.PadringTop))
	for _, pl := range res.SignalLocations.PadringTop {
		var typ *elaborate.ResolvedType
		if s := res.Signals[pl.Name]; s != nil {
			typ = s.Type
		}
		mark, con := typeToSubtype(typ)
		out = append(out, &vhdl.InterfaceDecl{Names: []string{pl.Name}, Mode: pl.Dir, SubtypeMark: mark, Constraint: con})
	}
	return out
}

// topInstStmt instantiates a top-entity: `configuration work.<cfg>` when resolved
// via a configuration, else `entity work.<entity>(<arch>)`. Builds a generic map
// from re.Generics (YAML keys lowercased; bool TRUE/FALSE casing deferred to
// P6b-3c) and a port map wiring each port to its global signal (no data-bus/subst
// at the top level). sortInstMaps sorts both maps before printing.
// Requires: if re.Config is nil, re.Entity must be non-nil (caller ensures binding).
// res is used to expand entity-bound inout ports (BufEntity) into per-bit pad associations.
func topInstStmt(re *elaborate.ResolvedEntity, res *elaborate.Resolution) *vhdl.InstantiationStmt {
	inst := &vhdl.InstantiationStmt{Label: lc(re.Name)}
	if re.Config != nil {
		inst.UnitKind = vhdl.CONFIGURATION
		inst.Unit = "work." + lc(re.Config.Name)
	} else {
		inst.UnitKind = vhdl.ENTITY
		inst.Unit = "work." + lc(re.Entity.Name)
		inst.Arch = re.ArchName
	}
	for name, val := range re.Generics {
		inst.GenericMap = append(inst.GenericMap, &vhdl.AssocElement{Formal: lc(name), Actual: emitValue(val)})
	}
	for _, p := range re.Ports {
		if res != nil && p.Kind == elaborate.KindSignal && res.EntityBoundPads[p.GlobalSignal] {
			for _, bit := range entityPadBits(res, p.GlobalSignal) {
				inst.PortMap = append(inst.PortMap, &vhdl.AssocElement{
					Formal: lc(p.Name) + "(" + bit.idx + ")",
					Actual: &vhdl.Ident{Name: "pin_" + bit.net},
				})
			}
			continue
		}
		inst.PortMap = append(inst.PortMap, &vhdl.AssocElement{Formal: lc(p.Name), Actual: portActual(p, "", nil, nil)})
	}
	return inst
}

// entityPadBits returns the per-bit pad entries for an entity-bound base signal,
// sorted ascending by numeric bit index. Each entry holds the bit index string and
// the net name (e.g. {idx:"0", net:"sdram_d0"}).
func entityPadBits(res *elaborate.Resolution, base string) []struct{ idx, net string } {
	type entry struct {
		n   int
		idx string
		net string
	}
	var entries []entry
	for _, pin := range res.Pins {
		if pin.BufferKind != elaborate.BufEntity || pin.Signal != base {
			continue
		}
		suffix := strings.TrimPrefix(pin.Net, base)
		n, err := strconv.Atoi(suffix)
		if err != nil {
			continue
		}
		entries = append(entries, entry{n: n, idx: suffix, net: pin.Net})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].n < entries[j].n })
	out := make([]struct{ idx, net string }, len(entries))
	for i, e := range entries {
		out[i] = struct{ idx, net string }{e.idx, e.net}
	}
	return out
}

// devicesInstStmt instantiates the devices architecture, wiring each TopDevices
// port to the same-named global signal (name => name).
func devicesInstStmt(res *elaborate.Resolution) *vhdl.InstantiationStmt {
	inst := &vhdl.InstantiationStmt{Label: "devices", UnitKind: vhdl.ENTITY, Unit: "work.devices", Arch: "impl"}
	if res.SignalLocations != nil {
		for _, pl := range res.SignalLocations.TopDevices {
			inst.PortMap = append(inst.PortMap, &vhdl.AssocElement{Formal: pl.Name, Actual: &vhdl.Ident{Name: pl.Name}})
		}
	}
	return inst
}
