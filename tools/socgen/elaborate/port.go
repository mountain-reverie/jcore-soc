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

// buildPorts resolves a device's ports: entity ports (name/dir/type) overlaid
// with the device's port spec (global-signal / value / special-kind), types
// resolved via the generics, normal ports auto-named and merge-renamed.
func buildPorts(devName string, ent *iface.Entity, spec map[string]design.Value, env map[string]int64, merge map[string]string) []*ResolvedPort {
	if ent == nil {
		return nil
	}
	out := make([]*ResolvedPort, 0, len(ent.Ports))
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
			// normal port, auto-name
			rp.GlobalSignal = mergeName(devName+"_"+p.Name, merge)
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
			rp.GlobalSignal = mergeName(v.Text, merge) // explicit signal name
		default: // KindInt/KindStr/KindFloat/KindBool -> constant value
			vv := v
			rp.Kind, rp.Value = KindValue, &vv
		}
		out = append(out, rp)
	}
	return out
}

func mergeName(s string, merge map[string]string) string {
	if t, ok := merge[s]; ok {
		return t
	}
	return s
}

func hasKey(m map[string]any, k string) bool { _, ok := m[k]; return ok }
