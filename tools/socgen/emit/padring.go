package emit

import (
	"errors"
	"sort"

	"github.com/j-core/jcore-soc/tools/socgen/elaborate"
	"github.com/j-core/jcore-soc/tools/socgen/vhdl"
)

// bufferGenericAttrs are the attributes that belong to the I/O buffer instances
// (P5d-b), not the pad port; they are excluded from pad-port attributes.
// Must stay in sync with bufferGenericOrder (iobufs.go), which emits this same
// set as the buffer generic map.
var bufferGenericAttrs = map[string]bool{"iostandard": true, "drive": true, "slew": true, "diff_term": true}

// sortedPins returns res.Pins sorted by net for deterministic emission.
func sortedPins(res *elaborate.Resolution) []*elaborate.ResolvedPin {
	ps := append([]*elaborate.ResolvedPin(nil), res.Pins...)
	sort.Slice(ps, func(i, j int) bool { return ps[i].Net < ps[j].Net })
	return ps
}

// padRingPorts builds the pad_ring entity ports: one `pin_<net> : <dir> std_logic`
// per resolved pin (both legs of a differential pair are separate pins).
func padRingPorts(res *elaborate.Resolution) []*vhdl.InterfaceDecl {
	pins := sortedPins(res)
	out := make([]*vhdl.InterfaceDecl, 0, len(pins))
	for _, p := range pins {
		dir := p.PadDir
		if dir == "" {
			dir = "in"
		}
		out = append(out, &vhdl.InterfaceDecl{Names: []string{"pin_" + p.Net}, Mode: dir, SubtypeMark: stdLogicMark})
	}
	return out
}

// pinAttrs builds the pad-port attribute declarations + specifications: `loc` (the
// pad) plus any non-buffer-generic attribute (e.g. tig). Buffer generics
// (iostandard/drive/slew/diff_term) belong to the I/O buffers (P5d-b).
func pinAttrs(res *elaborate.Resolution) []vhdl.Decl {
	pins := sortedPins(res)
	// collect distinct attribute names for the decls. `loc` is declared first
	// (only when at least one pin carries a pad), then the other attribute
	// names in first-seen/sorted order.
	var otherDecls []string
	hasLoc := false
	seen := map[string]bool{}
	type spec struct{ name, ent, val string }
	var specs []spec
	for _, p := range pins {
		port := "pin_" + p.Net
		if p.Pad != "" {
			hasLoc = true
			specs = append(specs, spec{"loc", port, vhdlEscape(p.Pad)})
		}
		// other attrs (sorted) excluding buffer generics and loc.
		var keys []string
		for k := range p.Attrs {
			lk := lc(k)
			if lk == "loc" || bufferGenericAttrs[lk] {
				continue
			}
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			lk := lc(k)
			if !seen[lk] {
				seen[lk] = true
				otherDecls = append(otherDecls, lk)
			}
			specs = append(specs, spec{lk, port, vhdlEscape(p.Attrs[k].Text)})
		}
	}
	declOrder := otherDecls
	if hasLoc {
		declOrder = append([]string{"loc"}, otherDecls...)
	}
	out := make([]vhdl.Decl, 0, len(declOrder)+len(specs))
	for _, name := range declOrder {
		out = append(out, &vhdl.AttributeDecl{Name: name, TypeMark: "string"})
	}
	for _, s := range specs {
		out = append(out, &vhdl.AttributeSpec{
			Name: s.name, Entities: []string{s.ent}, EntityClass: vhdl.SIGNAL,
			Value: &vhdl.BasicLit{Kind: vhdl.STRINGLIT, Value: `"` + s.val + `"`},
		})
	}
	return out
}

// socInstStmt instantiates the soc architecture inside pad_ring, wiring each
// PadringTop port to the same-named pad_ring signal (name => name).
func socInstStmt(res *elaborate.Resolution) *vhdl.InstantiationStmt {
	inst := &vhdl.InstantiationStmt{Label: "soc", UnitKind: vhdl.ENTITY, Unit: "work.soc", Arch: "impl"}
	if res.SignalLocations != nil {
		for _, pl := range res.SignalLocations.PadringTop {
			inst.PortMap = append(inst.PortMap, &vhdl.AssocElement{Formal: pl.Name, Actual: &vhdl.Ident{Name: pl.Name}})
		}
	}
	return inst
}

// PadRing renders pad_ring.vhd: the FPGA top entity (pin_<net> ports + LOC/TIG
// attributes) and an architecture instantiating soc + the padring entities,
// declaring the Padring internal signals, and wiring the pads — the soc instance,
// padring entities, the system.pio pi/po loopback (P5d-c), then the I/O buffers +
// direct-wires (P5d-b). Best-effort; never panics.
func PadRing(res *elaborate.Resolution) (string, error) {
	if res == nil {
		return "", nil
	}
	var errs []error

	decls := pinAttrs(res)
	if res.SignalLocations != nil {
		names := append([]string(nil), res.SignalLocations.Padring...)
		sort.Strings(names)
		for _, n := range names {
			var typ *elaborate.ResolvedType
			if s := res.Signals[n]; s != nil {
				typ = s.Type
			}
			mark, con := typeToSubtype(typ)
			decls = append(decls, &vhdl.SignalDecl{Names: []string{n}, SubtypeMark: mark, Constraint: con})
		}
	}

	stmts := []vhdl.Stmt{socInstStmt(res)}
	for _, name := range sortedTopNames(res.PadringEntities) {
		re := res.PadringEntities[name]
		if re.Entity == nil && re.Config == nil {
			errs = append(errs, &EmitError{Kind: ErrUnboundEntity, Inst: re.Name})
			continue
		}
		stmts = append(stmts, topInstStmt(re))
	}

	stmts = append(stmts, pioStatements(res)...)

	pinStmts, perr := pinStatements(res)
	if perr != nil {
		errs = append(errs, perr)
	}
	stmts = append(stmts, pinStmts...)

	ctx := padringContext(res)

	df := &vhdl.DesignFile{
		Context: ctx,
		Units: []vhdl.DesignUnit{
			&vhdl.EntityDecl{Name: "pad_ring", Ports: padRingPorts(res)},
			&vhdl.ArchitectureBody{Name: "impl", Entity: "pad_ring", Decls: decls, Stmts: stmts},
		},
	}
	return withBanner(vhdl.Print(df)), errors.Join(errs...)
}
