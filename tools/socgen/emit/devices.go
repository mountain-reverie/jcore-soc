package emit

import (
	"errors"
	"slices"
	"sort"

	"github.com/j-core/jcore-soc/tools/socgen/design"
	"github.com/j-core/jcore-soc/tools/socgen/elaborate"
	"github.com/j-core/jcore-soc/tools/socgen/iface"
	"github.com/j-core/jcore-soc/tools/socgen/vhdl"
)

// stdLogicMark is the VHDL std_logic type mark used throughout the emit package.
const stdLogicMark = "std_logic"

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
	stmts := make([]vhdl.Stmt, 0, len(res.Devices)+1)
	// Device instantiations, alphabetical by label (golden order), led by the
	// `Instantiate devices` section comment.
	devs := slices.Clone(res.Devices) // copy so the sort does not mutate res.Devices
	sort.Slice(devs, func(i, j int) bool { return lc(devs[i].Name) < lc(devs[j].Name) })
	insts := make([]vhdl.Stmt, 0, len(devs))
	for _, dev := range devs {
		rc := res.Classes[lc(dev.Class)]
		var ent, arch string
		var cfg *iface.Configuration
		if rc != nil && rc.Entity != nil {
			ent, arch = rc.Entity.Name, rc.ArchName
			cfg = rc.Config
		}
		if ent == "" {
			errs = append(errs, &EmitError{Kind: ErrUnboundEntity, Inst: dev.Name})
			continue
		}
		busLit := ""
		if dev.DataBus {
			busLit = devLit(dev.Name)
		}
		var portOv map[string]string
		var vecAgg vhdl.Expr
		if res.IRQ != nil {
			portOv = res.IRQ.PortOverrides[dev.Name]
			if vn, ok := res.IRQ.VectorNumbers[dev.Name]; ok {
				vecAgg = vectorNumbersAgg(vn)
			}
		}
		insts = append(insts, instStmt(lc(dev.Name), ent, arch, cfg, dev.Generics, dev.GenericTypes, dev.Ports, busLit, subst, portOv, vecAgg))
	}
	if len(insts) > 0 {
		stmts = append(stmts, &vhdl.Comment{Text: "Instantiate devices"})
		stmts = append(stmts, insts...)
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
	// IRQ OR-combine concurrent assignments (none for mimas_v2).
	stmts = append(stmts, irqAssigns(res.IRQ)...)

	if res.DataBus != nil {
		// Data-bus mux/decode statements precede device instantiations (golden
		// devices.vhd:88-95); the decls join the signal declarations.
		decls = append(decls, muxBusDecls(res)...)
		decls = append(decls, databusDecls(res)...)
		stmts = append(databusStmts(res), stmts...)
	}
	// irqs<cpu> signals are the final declarations before `begin` (golden).
	decls = append(decls, irqDecls(res.IRQ)...)
	context := deviceContext(res)

	df := &vhdl.DesignFile{
		Context: context,
		Units: []vhdl.DesignUnit{
			&vhdl.EntityDecl{Name: "devices", Ports: devicesEntityPorts(res)},
			&vhdl.ArchitectureBody{Name: "impl", Entity: "devices", Decls: decls, Stmts: stmts},
		},
	}
	sortInstMaps(df)
	out := withBanner(vhdl.Print(df))
	if len(res.BusWord) > 0 {
		out = phase2Devices(out, res)
	}
	return out, errors.Join(errs...)
}

// instStmt builds one device instantiation: `label : configuration work.<cfg>`
// when cfg is non-nil (entity/arch are then ignored), else
// `label : entity work.<entity>(<arch>)`, followed by the generic map + port map.
// busLit is the device's device_t literal (e.g. "DEV_UART0") for a data-bus
// participant, "" otherwise; it wires the device's KindDataBus ports to the
// shared devs_bus_i/o arrays. subst remaps a port's global signal to its
// DevicesExtra alias (sig_<name>) when present.
func instStmt(label, entity, arch string, cfg *iface.Configuration, generics map[string]design.Value, genTypes map[string]*elaborate.ResolvedType, ports []*elaborate.ResolvedPort, busLit string, subst map[string]string, portOv map[string]string, vecAgg vhdl.Expr) *vhdl.InstantiationStmt {
	var inst *vhdl.InstantiationStmt
	if cfg != nil {
		// A class that resolved to a configuration (e.g. emac -> eth_mac_rmii_fpga)
		// is instantiated as `configuration work.<cfg>` (Clojure instantiate-config),
		// mirroring topInstStmt.
		inst = &vhdl.InstantiationStmt{Label: label, UnitKind: vhdl.CONFIGURATION, Unit: "work." + lc(cfg.Name)}
	} else {
		inst = &vhdl.InstantiationStmt{Label: label, UnitKind: vhdl.ENTITY, Unit: "work." + lc(entity), Arch: arch}
	}
	// Build the generic map as (formal, actual) pairs so an injected expression
	// generic (vector_numbers, a vhdl.Expr rather than a design.Value) can be
	// sorted into alphabetical formal order alongside the design.Value generics.
	type genPair struct {
		formal string
		actual vhdl.Expr
	}
	gens := make([]genPair, 0, len(generics)+1)
	for _, g := range sortedKeys(generics) {
		// genTypes is keyed by entity generic name; a generic absent from the entity
		// (caught as ErrUnknownGeneric upstream) yields nil → numVal falls back to emitValue.
		key := lc(g)
		gens = append(gens, genPair{formal: key, actual: numVal(genTypes[key], generics[g])})
	}
	// vecAgg is the IRQ-network-derived vector_numbers. An explicit
	// vector_numbers generic in the design overrides it (e.g. boards whose IRQ
	// source is a raw pin, not an irq-declaring device), and avoids emitting the
	// generic twice.
	hasExplicitVectors := false
	for _, g := range sortedKeys(generics) {
		if lc(g) == "vector_numbers" {
			hasExplicitVectors = true
			break
		}
	}
	if vecAgg != nil && !hasExplicitVectors {
		gens = append(gens, genPair{formal: "vector_numbers", actual: vecAgg})
	}
	sort.Slice(gens, func(i, j int) bool { return gens[i].formal < gens[j].formal })
	for _, g := range gens {
		inst.GenericMap = append(inst.GenericMap, &vhdl.AssocElement{Formal: g.formal, Actual: g.actual})
	}
	for _, p := range ports {
		inst.PortMap = append(inst.PortMap, &vhdl.AssocElement{Formal: lc(p.Name), Actual: portActual(p, busLit, subst, portOv)})
	}
	return inst
}

// portActual maps a resolved port to its actual expression. busLit, when
// non-empty, is the device's device_t literal: a KindDataBus port is wired to the
// shared bus array element (db_o/"out" -> devs_bus_i(DEV_x), db_i/"in" ->
// devs_bus_o(DEV_x); generate.clj:609-612). subst remaps a KindSignal port's
// global signal to its DevicesExtra alias. IRQ/deferred ports remain placeholders
// wired in later sub-milestones (P5e IRQ).
func portActual(p *elaborate.ResolvedPort, busLit string, subst map[string]string, portOv map[string]string) vhdl.Expr {
	// IRQ wiring (P5e) overrides a port's actual outright: an explicit override
	// text (e.g. "irqs0(4)"|"irqs0"), or "open"/"" for an unrouted irq path.
	if ov, ok := portOv[p.Name]; ok {
		if ov == "" || ov == "open" {
			return &vhdl.Ident{Name: "open"}
		}
		return &vhdl.Ident{Name: ov}
	}
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
		return numVal(p.Type, *p.Value)
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
		return stdLogicMark, nil
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
