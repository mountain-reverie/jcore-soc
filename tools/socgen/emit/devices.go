package emit

import (
	"errors"
	"sort"

	"github.com/j-core/jcore-soc/tools/socgen/design"
	"github.com/j-core/jcore-soc/tools/socgen/elaborate"
	"github.com/j-core/jcore-soc/tools/socgen/vhdl"
)

// Devices renders devices.vhd from the elaborated model: a `devices` entity whose
// ports are the TopDevices boundary signals (P5c-i categorization), and an
// architecture `impl` declaring the Devices-category internal signals (+ DevicesExtra
// aliases + the data-bus infra) and instantiating ONLY the device instances. Top and
// padring entities are emitted in soc.vhd / pad_ring.vhd, not here. Best-effort;
// never panics; accumulates per-instance errors via errors.Join.
func Devices(res *elaborate.Resolution) (string, error) {
	if res == nil {
		return "", nil
	}
	var errs []error

	// Internal signal decls: only the Devices-category signals (P5c-i) plus the
	// readable aliases of DevicesExtra signals. subst maps a source signal name to
	// its alias so device output ports and the alias-driving assignments line up.
	var declNames []string
	subst := map[string]string{}
	if sl := res.SignalLocations; sl != nil {
		declNames = append(declNames, sl.Devices...)
		for name, alias := range sl.DevicesExtra {
			declNames = append(declNames, alias)
			subst[name] = alias
		}
		sort.Strings(declNames)
	} else { // fallback: no categorization -> old all-signals behavior
		for n := range res.Signals {
			declNames = append(declNames, n)
		}
		sort.Strings(declNames)
	}
	decls := make([]vhdl.Decl, 0, len(declNames))
	for _, n := range declNames {
		mark, con := typeToSubtype(typeForDecl(res, n, subst))
		decls = append(decls, &vhdl.SignalDecl{Names: []string{n}, SubtypeMark: mark, Constraint: con})
	}

	// Statements: device instantiations only. Top/padring entities move to
	// soc.vhd / pad_ring in a later sub-milestone (P5a correction).
	stmts := make([]vhdl.Stmt, 0, len(res.Devices))
	for _, dev := range res.Devices {
		rc := res.Classes[lc(dev.Class)]
		var ent, arch string
		if rc != nil && rc.Entity != nil {
			ent, arch = rc.Entity.Name, rc.ArchName
		}
		if ent == "" {
			errs = append(errs, &EmitError{Kind: ErrUnboundEntity, Inst: dev.Name})
			continue
		}
		busLit := ""
		if dev.DataBus {
			busLit = devLit(dev.Name)
		}
		stmts = append(stmts, instStmt(lc(dev.Name), ent, arch, dev.Generics, dev.Ports, busLit, subst))
	}
	// DevicesExtra: drive each output port from its readable alias (name <= sig_name).
	substKeys := make([]string, 0, len(subst))
	for name := range subst {
		substKeys = append(substKeys, name)
	}
	sort.Strings(substKeys)
	for _, name := range substKeys {
		stmts = append(stmts, concAssign(&vhdl.Ident{Name: name}, &vhdl.Ident{Name: subst[name]}))
	}

	context := []vhdl.Node{
		&vhdl.LibraryClause{Names: []string{"ieee"}},
		&vhdl.UseClause{Names: []string{"ieee.std_logic_1164.all"}},
	}
	if res.DataBus != nil {
		// Data-bus mux/decode statements precede device instantiations (golden
		// devices.vhd:88-95); the decls join the signal declarations.
		decls = append(decls, databusDecls(res)...)
		stmts = append(databusStmts(res), stmts...)
		context = append(context, &vhdl.UseClause{Names: []string{"work.data_bus_pack.all"}})
	}

	df := &vhdl.DesignFile{
		Context: context,
		Units: []vhdl.DesignUnit{
			&vhdl.EntityDecl{Name: "devices", Ports: devicesEntityPorts(res)},
			&vhdl.ArchitectureBody{Name: "impl", Entity: "devices", Decls: decls, Stmts: stmts},
		},
	}
	return vhdl.Print(df), errors.Join(errs...)
}

// instStmt builds one `label : entity work.<entity>(<arch>) generic map(...) port map(...)`.
// busLit is the device's device_t literal (e.g. "DEV_UART0") for a data-bus
// participant, "" otherwise; it wires the device's KindDataBus ports to the
// shared devs_bus_i/o arrays. subst remaps a port's global signal to its
// DevicesExtra alias (sig_<name>) when present.
func instStmt(label, entity, arch string, generics map[string]design.Value, ports []*elaborate.ResolvedPort, busLit string, subst map[string]string) *vhdl.InstantiationStmt {
	inst := &vhdl.InstantiationStmt{Label: label, UnitKind: vhdl.ENTITY, Unit: "work." + lc(entity), Arch: arch}
	for _, g := range sortedKeys(generics) {
		inst.GenericMap = append(inst.GenericMap, &vhdl.AssocElement{Formal: lc(g), Actual: emitValue(generics[g])})
	}
	for _, p := range ports {
		inst.PortMap = append(inst.PortMap, &vhdl.AssocElement{Formal: lc(p.Name), Actual: portActual(p, busLit, subst)})
	}
	return inst
}

// portActual maps a resolved port to its actual expression. busLit, when
// non-empty, is the device's device_t literal: a KindDataBus port is wired to the
// shared bus array element (db_o/"out" -> devs_bus_i(DEV_x), db_i/"in" ->
// devs_bus_o(DEV_x); generate.clj:609-612). subst remaps a KindSignal port's
// global signal to its DevicesExtra alias. IRQ/deferred ports remain placeholders
// wired in later sub-milestones (P5e IRQ).
func portActual(p *elaborate.ResolvedPort, busLit string, subst map[string]string) vhdl.Expr {
	switch p.Kind {
	case elaborate.KindSignal:
		gs := p.GlobalSignal
		if a, ok := subst[gs]; ok {
			gs = a
		}
		if gs == "" {
			return &vhdl.Ident{Name: "open"}
		}
		return &vhdl.Ident{Name: gs}
	case elaborate.KindValue:
		if p.Value == nil {
			return &vhdl.Ident{Name: "open"}
		}
		return emitValue(*p.Value)
	case elaborate.KindDataBus:
		if busLit == "" {
			return &vhdl.Ident{Name: "open"}
		}
		arr := "devs_bus_o"
		if p.Dir == "out" {
			arr = "devs_bus_i"
		}
		return &vhdl.CallExpr{Fun: &vhdl.Ident{Name: arr}, Args: []*vhdl.AssocElement{{Actual: &vhdl.Ident{Name: busLit}}}}
	case elaborate.KindIRQ, elaborate.KindDeferred:
		// Placeholders wired in later sub-milestones (P5e IRQ).
		return &vhdl.Ident{Name: "open"}
	default:
		// Defensive: an unrecognized port kind degrades to open rather than
		// emitting an invalid port map.
		return &vhdl.Ident{Name: "open"}
	}
}

// devicesEntityPorts builds the devices entity's port list from the TopDevices
// boundary signals (P5c-i categorization); each port's type comes from its signal.
func devicesEntityPorts(res *elaborate.Resolution) []*vhdl.InterfaceDecl {
	if res.SignalLocations == nil {
		return nil
	}
	out := make([]*vhdl.InterfaceDecl, 0, len(res.SignalLocations.TopDevices))
	for _, pl := range res.SignalLocations.TopDevices {
		var typ *elaborate.ResolvedType
		if s := res.Signals[pl.Name]; s != nil {
			typ = s.Type
		}
		mark, con := typeToSubtype(typ)
		out = append(out, &vhdl.InterfaceDecl{Names: []string{pl.Name}, Mode: pl.Dir, SubtypeMark: mark, Constraint: con})
	}
	return out
}

// typeForDecl returns the type for an internal signal decl; a DevicesExtra alias
// (sig_<name>) inherits the type of its source signal <name>.
func typeForDecl(res *elaborate.Resolution, declName string, subst map[string]string) *elaborate.ResolvedType {
	src := declName
	for name, alias := range subst {
		if alias == declName {
			src = name
			break
		}
	}
	if s := res.Signals[src]; s != nil {
		return s.Type
	}
	return nil
}

// typeToSubtype renders a resolved signal type to a (mark, constraint) pair.
// Concrete vector bounds (Left/Right) are emitted as an index constraint; a
// symbolic type keeps its as-written constraint; a nil type defaults to std_logic.
func typeToSubtype(t *elaborate.ResolvedType) (string, vhdl.Expr) {
	if t == nil {
		return "std_logic", nil
	}
	if t.Left != nil && t.Right != nil {
		dir := vhdl.TO
		if t.Dir == "downto" {
			dir = vhdl.DOWNTO
		}
		return t.Mark, &vhdl.ParenExpr{X: &vhdl.Range{Left: intLit(*t.Left), Dir: dir, Right: intLit(*t.Right)}}
	}
	return t.Mark, t.Constraint
}
