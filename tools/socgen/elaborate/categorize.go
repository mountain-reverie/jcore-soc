package elaborate

import (
	"errors"
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

// Position sets for the categories (generate.clj:1240-1306). P = pin/padring/expose.
var (
	catP  = []string{"pin", "padring", "expose"}
	catPT = []string{"pin", "padring", "expose", "top"}
	catTD = []string{"top", "device"}
)

// Boundary direction maps from a signal's source context (generate.clj:1262-1276).
var padringTopDir = map[string]string{"pin": dirIn, "padring": dirIn, "expose": dirIn, "device": dirOut, "top": dirOut}
var topDevicesDir = map[string]string{"pin": dirIn, "padring": dirIn, "expose": dirIn, "device": dirOut, "top": dirIn}

// categorize partitions the (already injected) net-list into SignalLocations
// (generate.clj categorize-signals).
func categorize(res *Resolution) (*SignalLocations, error) {
	cs := contextSets(res.Signals)
	var errs []error

	ports := func(sigs []*Signal, dirOf map[string]string) []PortLoc {
		out := make([]PortLoc, 0, len(sigs))
		for _, s := range sigs {
			src, err := srcContext(s)
			if err != nil {
				errs = append(errs, err)
				continue
			}
			out = append(out, PortLoc{Name: s.Name, Dir: dirOf[src]})
		}
		sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
		return out
	}
	nameList := func(sigs []*Signal) []string {
		out := make([]string, 0, len(sigs))
		for _, s := range sigs {
			out = append(out, s.Name)
		}
		sort.Strings(out)
		return out
	}
	aliasMap := func(sigs []*Signal) map[string]string {
		m := map[string]string{}
		for _, s := range sigs {
			m[s.Name] = "sig_" + s.Name
		}
		return m
	}

	padringTop := filterByContext(cs, catP, catP, catP, catTD, catTD)
	topDevices := filterByContext(cs, catPT, catPT, catPT, catPT, []string{"device"})

	sl := &SignalLocations{
		PadringTop:   ports(padringTop, padringTopDir),
		TopDevices:   ports(topDevices, topDevicesDir),
		Padring:      nameList(filterByContext(cs, catP, catP, catP, []string{"top", "device", ""}, []string{"top", "device", ""})),
		Top:          nameList(filterByContext(cs, []string{"top"}, []string{"device", ""})),
		Devices:      nameList(filterByContext(cs, []string{"device"})),
		TopExtra:     aliasMap(inOutSignals(padringTop, "top")),
		DevicesExtra: aliasMap(inOutSignals(topDevices, "device")),
	}
	return sl, errors.Join(errs...)
}

// injectInternalBusPorts adds synthetic device/_internal ports for each peripheral
// bus + the pi hack, so categorization classifies them as devices/soc boundary
// ports (devices.clj:885-912). Called before categorize when DataBus != nil.
func injectInternalBusPorts(res *Resolution) {
	if res.DataBus == nil {
		return
	}
	buses := append(append([]string{}, res.DataBus.Connected...), res.DataBus.Disconnected...)
	sort.Strings(buses)
	addPort := func(name, mark, dir string, ctx Context) {
		s := res.Signals[name]
		if s == nil {
			s = &Signal{Name: name, Type: &ResolvedType{Mark: mark}}
			res.Signals[name] = s
		}
		s.Ports = append(s.Ports, &SignalPortRef{Context: ctx, PortName: name, Dir: dir, Type: s.Type})
	}
	devInternal := Context{Kind: "device", ID: "_internal"}
	for _, bus := range buses {
		addPort(bus+"_periph_dbus_i", "cpu_data_i_t", dirOut, devInternal) // devices outputs read data
		addPort(bus+"_periph_dbus_o", "cpu_data_o_t", dirIn, devInternal)  // devices inputs write data
	}
	// pi hack (devices.clj:903-912 adds a padring port for "pi"; the reference's
	// :in works only because its net-list has a pin driving pi). In the Go model
	// mimas_v2 has no pi input pins, so pi is a pure primary input: the padring
	// port must be the DRIVER (out) — padring drives pi into the devices — so that
	// it has a source context and categorizes as a devices/soc input port.
	if s := res.Signals["pi"]; s != nil {
		hasDevIn, alreadyDriven := false, false
		for _, p := range s.Ports {
			if p.Context.Kind == "device" && p.Dir == dirIn {
				hasDevIn = true
			}
			if p.Dir == dirOut { // a real driver (e.g. a pi input pin) exists
				alreadyDriven = true
			}
		}
		// Only synthesize the padring driver when pi is consumed by a device and not
		// already driven — avoids a two-driver pi on boards that DO have pi input pins.
		if hasDevIn && !alreadyDriven {
			s.Ports = append(s.Ports, &SignalPortRef{Context: Context{Kind: "padring", ID: "pi"}, PortName: "pi", Dir: dirOut, Type: s.Type})
		}
	}
}
