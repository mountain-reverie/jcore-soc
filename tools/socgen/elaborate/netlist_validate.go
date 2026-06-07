package elaborate

import (
	"errors"
	"sort"
	"strings"
)

// validateSignals checks each global signal for type consistency, a single
// driver, and that consumed signals are driven. Best-effort; appends errors.
func validateSignals(sigs map[string]*Signal) error {
	var errs []error
	names := make([]string, 0, len(sigs))
	for n := range sigs {
		names = append(names, n)
	}
	sort.Strings(names) // deterministic error order
	for _, n := range names {
		s := sigs[n]
		// type consistency
		types := map[string]bool{}
		for _, p := range s.Ports {
			types[p.Type.String()] = true
		}
		if len(types) > 1 {
			errs = append(errs, &SignalError{Kind: ErrTypeMismatch, Signal: n, Detail: strings.Join(sortedKeys(types), " vs ")})
		}
		// single driver. A port drives if out/buffer/inout; it consumes if in/inout.
		var drivers []*SignalPortRef
		var ins []string
		for _, p := range s.Ports {
			if isDriver(p.Dir) {
				drivers = append(drivers, p)
			}
			if isConsumer(p.Dir) {
				ins = append(ins, p.Context.ID+"."+p.PortName)
			}
		}
		if len(drivers) > 1 && !multiDriverAllowed(drivers) {
			driverNames := make([]string, len(drivers))
			for i, d := range drivers {
				driverNames[i] = d.Context.ID + "." + d.PortName
			}
			errs = append(errs, &SignalError{Kind: ErrMultiDriver, Signal: n, Detail: strings.Join(driverNames, " ")})
		}
		if len(drivers) == 0 && len(ins) > 0 {
			errs = append(errs, &SignalError{Kind: ErrUndrivenSignal, Signal: n, Detail: strings.Join(ins, " ")})
		}
	}
	return errors.Join(errs...)
}

// isDriver reports whether a port direction drives its signal (a source).
func isDriver(dir string) bool {
	switch dir {
	case dirOut, dirBuffer, dirInout:
		return true
	}
	return false
}

// isConsumer reports whether a port direction consumes its signal (a sink).
func isConsumer(dir string) bool {
	switch dir {
	case dirIn, dirInout:
		return true
	}
	return false
}

// multiDriverAllowed permits >1 driver in two pin-only cases: a differential
// pos/neg pair (exactly two), or every driver targeting a distinct bus element.
// All drivers must be pin-context for either exception to apply.
func multiDriverAllowed(drivers []*SignalPortRef) bool {
	for _, d := range drivers {
		if d.Context.Kind != ctxKindPin {
			return false
		}
	}
	// differential pair: exactly two, one pos and one neg (differential pins are
	// whole-signal, so Element is expected to be "" and is not consulted here)
	if len(drivers) == 2 {
		diffs := map[string]bool{}
		for _, d := range drivers {
			diffs[d.Diff] = true
		}
		if diffs["pos"] && diffs["neg"] && len(diffs) == 2 {
			return true
		}
	}
	// distinct bus elements: every driver targets a different non-empty element
	seen := map[string]bool{}
	for _, d := range drivers {
		if d.Element == "" || seen[d.Element] {
			return false
		}
		seen[d.Element] = true
	}
	return true
}

func sortedKeys(m map[string]bool) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}
