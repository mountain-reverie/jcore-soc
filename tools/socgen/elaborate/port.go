package elaborate

import (
	"github.com/j-core/jcore-soc/tools/socgen/design"
	"github.com/j-core/jcore-soc/tools/socgen/iface"
)

// genericEnv builds the integer environment for type-bound evaluation from a
// device's effective generics (KindInt values) plus any entity-default generics
// that are integer literals (lower priority).
func genericEnv(generics map[string]design.Value, ent *iface.Entity) map[string]int64 {
	env := map[string]int64{}
	if ent != nil {
		for _, g := range ent.Generics {
			if g.Default != nil {
				if v, ok := evalInt(g.Default, env); ok {
					env[lc(g.Name)] = v
				}
			}
		}
	}
	for name, v := range generics { // device generics override defaults
		if v.Kind == design.KindInt {
			env[lc(name)] = v.Int
		}
	}
	return env
}

// reverseMerge turns {target: [aliases...]} into {alias: target}.
func reverseMerge(m map[string][]string) map[string]string {
	out := map[string]string{}
	for target, aliases := range m {
		for _, a := range aliases {
			out[a] = target
		}
	}
	return out
}

// buildPorts resolves a device's or entity's ports: entity ports (name/dir/type)
// overlaid with the port spec (global-signal / value / special-kind), types
// resolved via the generics, normal ports auto-named and merge-renamed. When
// bareDefault is true (top/padring-entities), an unmapped port's default
// global-signal is the bare port id so it unifies with shared/device signals;
// otherwise (devices) it is devName_portName.
func buildPorts(devName string, ent *iface.Entity, spec map[string]design.Value, env map[string]int64, merge map[string]string, bareDefault bool, lib *iface.Library) []*ResolvedPort {
	if ent == nil {
		return nil
	}
	out := make([]*ResolvedPort, 0, len(ent.Ports))
	explicit := map[string]bool{}
	for _, p := range ent.Ports {
		rp := &ResolvedPort{
			Name: p.Name,
			Dir:  p.Dir,
			Type: resolveType(p.Type.Mark, p.Type.Constraint, env),
			Kind: KindSignal,
		}
		v, has := spec[p.Name]
		switch {
		case !has:
			switch {
			case p.GlobalName != "":
				// soc_port_global_name / global_ports group: a bare SoC-wide signal.
				rp.GlobalSignal = mergeName(p.GlobalName, merge)
				explicit[p.Name] = true // not subject to the clk/rst heuristic / further default
			default:
				// auto-name: localName (soc_port_local_name / local_ports) overrides the
				// port id in the prefix; bare id for top/padring, devName-prefixed for devices.
				local := p.Name
				if p.LocalName != "" {
					local = p.LocalName
				}
				def := devName + "_" + local
				if bareDefault {
					def = local
				}
				rp.GlobalSignal = mergeName(def, merge)
			}
		case v.Kind == design.KindMap:
			switch {
			case hasKey(v.Map, "irq?"):
				rp.Kind = KindIRQ
			case hasKey(v.Map, "data-bus"):
				rp.Kind = KindDataBus
			default:
				rp.Kind = KindDeferred // bist-chain/ring-bus/open? — recorded only (P4d/P5)
			}
		case v.Kind == design.KindExpr:
			if lib != nil && lib.IsConstant(v.Text) {
				// A library constant: rendered as the identifier actual, not
				// declared as a signal (faithful to the Clojure :value classification).
				vv := v
				rp.Kind, rp.Value = KindValue, &vv
			} else {
				rp.GlobalSignal = mergeName(v.Text, merge) // explicit signal name
				explicit[p.Name] = true
			}
		default: // KindInt/KindStr/KindFloat/KindBool -> constant value
			vv := v
			rp.Kind, rp.Value = KindValue, &vv
		}
		// A soc_port_irq output (e.g. gpio.irq, uart.int) is an interrupt port:
		// its actual is wired by the IRQ model (P5e). Only when the device didn't
		// give it an explicit signal/value.
		if p.IRQ && rp.Kind == KindSignal && !explicit[p.Name] {
			rp.Kind = KindIRQ
			rp.GlobalSignal = "" // IRQ ports are wired by the IRQ model, not via GlobalSignal
		}
		out = append(out, rp)
	}
	applyClkRstHeuristic(out, explicit, merge)
	return out
}

// applyClkRstHeuristic maps a unique clk/clk_bus port to clk_sys and a unique
// rst/reset port to reset (faithful to parse.clj find-sig-ports). It only rewrites
// a normal KindSignal port that was NOT explicitly mapped, and only when the
// target signal is not already another port's GlobalSignal.
func applyClkRstHeuristic(ports []*ResolvedPort, explicit map[string]bool, merge map[string]string) {
	apply := func(target string, cands ...string) {
		var cand *ResolvedPort
		n := 0
		for _, p := range ports {
			if p.Kind != KindSignal || explicit[p.Name] {
				continue
			}
			for _, c := range cands {
				if lc(p.Name) == c {
					n++
					cand = p
				}
			}
		}
		if n != 1 {
			return
		}
		tgt := mergeName(target, merge)
		for _, p := range ports {
			if p.Kind == KindSignal && !explicit[p.Name] && p.GlobalSignal == tgt {
				return // target already used by another non-explicit port
			}
		}
		cand.GlobalSignal = tgt
	}
	apply("clk_sys", "clk", "clk_bus")
	apply("reset", "rst", "reset")
}

func mergeName(s string, merge map[string]string) string {
	if t, ok := merge[s]; ok {
		return t
	}
	return s
}

func hasKey(m map[string]any, k string) bool { _, ok := m[k]; return ok }
