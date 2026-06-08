package emit

import (
	"github.com/j-core/jcore-soc/tools/socgen/elaborate"
	"github.com/j-core/jcore-soc/tools/socgen/vhdl"
)

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
// via a configuration, else `entity work.<entity>(<arch>)`. Ports wire to their
// global signals (no data-bus/subst at the top level). ResolvedEntity carries no
// generics, so the generic map is omitted (a later refinement; the golden has it).
// Requires: if re.Config is nil, re.Entity must be non-nil (caller ensures binding).
func topInstStmt(re *elaborate.ResolvedEntity) *vhdl.InstantiationStmt {
	inst := &vhdl.InstantiationStmt{Label: lc(re.Name)}
	if re.Config != nil {
		inst.UnitKind = vhdl.CONFIGURATION
		inst.Unit = "work." + lc(re.Config.Name)
	} else {
		inst.UnitKind = vhdl.ENTITY
		inst.Unit = "work." + lc(re.Entity.Name)
		inst.Arch = re.ArchName
	}
	for _, p := range re.Ports {
		inst.PortMap = append(inst.PortMap, &vhdl.AssocElement{Formal: lc(p.Name), Actual: portActual(p, "", nil)})
	}
	return inst
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
