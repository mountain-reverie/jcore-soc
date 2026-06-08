package emit

import (
	"errors"
	"sort"

	"github.com/j-core/jcore-soc/tools/socgen/design"
	"github.com/j-core/jcore-soc/tools/socgen/elaborate"
	"github.com/j-core/jcore-soc/tools/socgen/vhdl"
)

// Devices renders the structural devices.vhd from the elaborated model: an entity
// `devices` (no ports in P5a — signal categorization is P5c) and an architecture
// `impl` declaring every net-list signal and instantiating every device + top/
// padring entity with generic+port maps. Best-effort; never panics; accumulates
// per-instance errors via errors.Join.
func Devices(res *elaborate.Resolution) (string, error) {
	if res == nil {
		return "", nil
	}
	var errs []error

	names := make([]string, 0, len(res.Signals))
	for n := range res.Signals {
		names = append(names, n)
	}
	sort.Strings(names)
	decls := make([]vhdl.Decl, 0, len(names))
	for _, n := range names {
		mark, con := typeToSubtype(res.Signals[n].Type)
		decls = append(decls, &vhdl.SignalDecl{Names: []string{n}, SubtypeMark: mark, Constraint: con})
	}

	stmts := make([]vhdl.Stmt, 0, len(res.Devices)+len(res.TopEntities)+len(res.PadringEntities))
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
		stmts = append(stmts, instStmt(lc(dev.Name), ent, arch, dev.Generics, dev.Ports, busLit))
	}
	for _, name := range sortedEntityNames(res.TopEntities) {
		stmts = appendEntity(stmts, res.TopEntities[name], &errs)
	}
	for _, name := range sortedEntityNames(res.PadringEntities) {
		stmts = appendEntity(stmts, res.PadringEntities[name], &errs)
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
			&vhdl.EntityDecl{Name: "devices"},
			&vhdl.ArchitectureBody{Name: "impl", Entity: "devices", Decls: decls, Stmts: stmts},
		},
	}
	return vhdl.Print(df), errors.Join(errs...)
}

// appendEntity adds an instantiation for a resolved top/padring entity, or records
// an ErrUnboundEntity (and skips it) when the entity is unbound. It returns the
// extended statement slice and accumulates errors via the errs pointer.
func appendEntity(stmts []vhdl.Stmt, re *elaborate.ResolvedEntity, errs *[]error) []vhdl.Stmt {
	if re.Entity == nil {
		*errs = append(*errs, &EmitError{Kind: ErrUnboundEntity, Inst: re.Name})
		return stmts
	}
	return append(stmts, instStmt(lc(re.Name), re.Entity.Name, re.ArchName, nil, re.Ports, ""))
}

// instStmt builds one `label : entity work.<entity>(<arch>) generic map(...) port map(...)`.
// busLit is the device's device_t literal (e.g. "DEV_UART0") for a data-bus
// participant, "" otherwise; it wires the device's KindDataBus ports to the
// shared devs_bus_i/o arrays.
func instStmt(label, entity, arch string, generics map[string]design.Value, ports []*elaborate.ResolvedPort, busLit string) *vhdl.InstantiationStmt {
	inst := &vhdl.InstantiationStmt{Label: label, UnitKind: vhdl.ENTITY, Unit: "work." + lc(entity), Arch: arch}
	for _, g := range sortedKeys(generics) {
		inst.GenericMap = append(inst.GenericMap, &vhdl.AssocElement{Formal: lc(g), Actual: emitValue(generics[g])})
	}
	for _, p := range ports {
		inst.PortMap = append(inst.PortMap, &vhdl.AssocElement{Formal: lc(p.Name), Actual: portActual(p, busLit)})
	}
	return inst
}

// portActual maps a resolved port to its actual expression. busLit, when
// non-empty, is the device's device_t literal: a KindDataBus port is wired to the
// shared bus array element (db_o/"out" -> devs_bus_i(DEV_x), db_i/"in" ->
// devs_bus_o(DEV_x); generate.clj:609-612). IRQ/deferred ports remain placeholders
// wired in later sub-milestones (P5e IRQ).
func portActual(p *elaborate.ResolvedPort, busLit string) vhdl.Expr {
	switch p.Kind {
	case elaborate.KindSignal:
		if p.GlobalSignal == "" {
			return &vhdl.Ident{Name: "open"}
		}
		return &vhdl.Ident{Name: p.GlobalSignal}
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

// sortedEntityNames returns map keys sorted. (Duplicates a private helper in the
// elaborate package; the one-way package layering prevents sharing it.)
func sortedEntityNames(m map[string]*elaborate.ResolvedEntity) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}
