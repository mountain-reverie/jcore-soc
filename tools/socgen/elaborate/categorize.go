package elaborate

import (
	"sort"
	"strings"
)

// setKey is the deterministic key for a set of context kinds.
func setKey(set map[string]bool) string {
	ks := make([]string, 0, len(set))
	for k := range set {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return strings.Join(ks, ",")
}

// contextSets groups signals by the set of distinct port Context.Kind they touch
// (generate.clj:1363-1368). Iterates sorted for determinism.
func contextSets(sigs map[string]*Signal) map[string][]*Signal {
	out := map[string][]*Signal{}
	names := make([]string, 0, len(sigs))
	for n := range sigs {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		s := sigs[n]
		set := map[string]bool{}
		for _, p := range s.Ports {
			set[p.Context.Kind] = true
		}
		k := setKey(set)
		out[k] = append(out[k], s)
	}
	return out
}

// typeCombinations returns the cartesian product of positions as sets of kinds
// (empty-string picks dropped), deduped (generate.clj type-combinations).
func typeCombinations(positions ...[]string) []map[string]bool {
	combos := []map[string]bool{{}}
	for _, pos := range positions {
		var next []map[string]bool
		for _, c := range combos {
			for _, opt := range pos {
				nc := map[string]bool{}
				for k := range c {
					nc[k] = true
				}
				if opt != "" {
					nc[opt] = true
				}
				next = append(next, nc)
			}
		}
		combos = next
	}
	seen := map[string]bool{}
	var out []map[string]bool
	for _, c := range combos {
		k := setKey(c)
		if !seen[k] {
			seen[k] = true
			out = append(out, c)
		}
	}
	return out
}

// filterByContext concatenates signals whose context-set equals one of the
// type-combinations of positions (generate.clj filter-signals-by-context).
func filterByContext(cs map[string][]*Signal, positions ...[]string) []*Signal {
	var out []*Signal
	for _, combo := range typeCombinations(positions...) {
		out = append(out, cs[setKey(combo)]...)
	}
	return out
}

// srcContext returns the signal's single output-driver context kind (pin→padring);
// errors if not exactly one (generate.clj find-src-context).
func srcContext(s *Signal) (string, error) {
	set := map[string]bool{}
	for _, p := range s.Ports {
		if p.Dir != dirOut {
			continue
		}
		k := p.Context.Kind
		if k == ctxKindPin {
			k = "padring"
		}
		set[k] = true
	}
	if len(set) != 1 {
		return "", &SignalError{Kind: ErrMultiContextDriver, Signal: s.Name, Detail: setKey(set)}
	}
	for k := range set {
		return k, nil
	}
	panic("unreachable: set has exactly one element")
}

// inOutSignals returns signals whose ports within ctx have dirs exactly {in,out}
// (generate.clj find-in-out-signals).
func inOutSignals(sigs []*Signal, ctx string) []*Signal {
	var out []*Signal
	for _, s := range sigs {
		dirs := map[string]bool{}
		for _, p := range s.Ports {
			if p.Context.Kind == ctx {
				dirs[p.Dir] = true
			}
		}
		if len(dirs) == 2 && dirs[dirIn] && dirs[dirOut] {
			out = append(out, s)
		}
	}
	return out
}
