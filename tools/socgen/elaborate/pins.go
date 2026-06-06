package elaborate

import (
	"maps"
	"regexp"
	"strconv"

	"github.com/j-core/jcore-soc/tools/socgen/design"
)

// matchPin returns the captured symbol->int environment if rule matches pin net.
// A regex match is full-anchored; a parametric match builds a regex from the
// rule's Parts (literals escaped, symbols -> ([0-9]+) captures).
func matchPin(rule *design.PinRule, net string) (map[string]int, bool) {
	m := rule.Match
	if m == nil {
		return nil, false
	}
	if len(m.Parts) == 0 {
		re, err := regexp.Compile("^(?:" + m.Regex + ")$")
		if err != nil {
			return nil, false
		}
		return map[string]int{}, re.MatchString(net)
	}
	pat := "^"
	var syms []string
	for _, p := range m.Parts {
		if p.Sym != "" {
			pat += "([0-9]+)"
			syms = append(syms, p.Sym)
		} else {
			pat += regexp.QuoteMeta(p.Lit)
		}
	}
	pat += "$"
	re, err := regexp.Compile(pat)
	if err != nil {
		return nil, false
	}
	g := re.FindStringSubmatch(net)
	if g == nil {
		return nil, false
	}
	env := map[string]int{}
	for i, s := range syms {
		env[s], _ = strconv.Atoi(g[i+1])
	}
	return env, true
}

// expandSig resolves a SigSpec to a concrete signal-ref string given the match
// env and the pin's net. The returned kind is SigConst for constant targets,
// else SigName (the ref is a fully-resolved name; sub-signal splitting is later).
func expandSig(s *design.SigSpec, pinNet string, env map[string]int) (ref, diff string, kind design.SigKind) {
	if s == nil {
		return "", "", design.SigName
	}
	switch s.Kind {
	case design.SigTrue:
		return pinNet, "", design.SigName
	case design.SigConst:
		return "", "", design.SigConst
	case design.SigMap:
		return s.Name, s.Diff, design.SigName
	case design.SigTemplate:
		out := ""
		for _, p := range s.Parts {
			if p.Sym != "" {
				out += strconv.Itoa(env[p.Sym])
			} else {
				out += p.Lit
			}
		}
		return out, "", design.SigName
	default: // SigName
		return s.Name, "", design.SigName
	}
}

// folded is the accumulated effect of all rules matching one pin.
type folded struct {
	attrs      map[string]design.Value
	buff       *bool
	signalRef  string // bare signal: (direction auto-inferred later)
	signalDiff string
	inRef      string
	outRef     string
	outEnRef   string
	hasConst   bool // a SigConst target (e.g. out: 0)
}

// foldRules applies every matching rule to pin in order: attrs accumulate, and
// signal/in/out/out-en/buff take the last matching rule's value (expanded with
// that rule's capture env).
func foldRules(rules []*design.PinRule, pin *design.Pin) folded {
	f := folded{attrs: map[string]design.Value{}}
	for _, r := range rules {
		env, ok := matchPin(r, pin.Net)
		if !ok {
			continue
		}
		maps.Copy(f.attrs, r.Attrs)
		if r.Buff != nil {
			f.buff = r.Buff
		}
		if r.Signal != nil {
			ref, diff, kind := expandSig(r.Signal, pin.Net, env)
			f.signalRef, f.signalDiff = ref, diff
			f.hasConst = f.hasConst || kind == design.SigConst
		}
		if r.In != nil {
			ref, _, kind := expandSig(r.In, pin.Net, env)
			f.inRef = ref
			f.hasConst = f.hasConst || kind == design.SigConst
		}
		if r.Out != nil {
			ref, _, kind := expandSig(r.Out, pin.Net, env)
			f.outRef = ref
			f.hasConst = f.hasConst || kind == design.SigConst
		}
		if r.OutEn != nil {
			ref, _, kind := expandSig(r.OutEn, pin.Net, env)
			f.outEnRef = ref
			f.hasConst = f.hasConst || kind == design.SigConst
		}
	}
	return f
}
