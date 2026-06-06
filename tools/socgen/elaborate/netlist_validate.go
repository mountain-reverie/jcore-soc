package elaborate

import (
	"fmt"
	"sort"
	"strings"
)

// validateSignals checks each global signal for type consistency, a single
// driver, and that consumed signals are driven. Best-effort; appends errors.
func validateSignals(sigs map[string]*Signal, errs []error) []error {
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
			errs = append(errs, fmt.Errorf("type mismatch for signal %q: %s", n, strings.Join(sortedKeys(types), " vs ")))
		}
		// single driver. A port drives if out/buffer/inout; it consumes if in/inout.
		var outs, ins []string
		for _, p := range s.Ports {
			if isDriver(p.Dir) {
				outs = append(outs, p.Context.ID+"."+p.PortName)
			}
			if isConsumer(p.Dir) {
				ins = append(ins, p.Context.ID+"."+p.PortName)
			}
		}
		if len(outs) > 1 {
			errs = append(errs, fmt.Errorf("signal %q is driven by multiple ports: %s", n, strings.Join(outs, " ")))
		}
		if len(outs) == 0 && len(ins) > 0 {
			errs = append(errs, fmt.Errorf("nothing drives signal %q used by %s", n, strings.Join(ins, " ")))
		}
	}
	return errs
}

// isDriver reports whether a port direction drives its signal (a source).
func isDriver(dir string) bool {
	switch dir {
	case "out", "buffer", "inout":
		return true
	}
	return false
}

// isConsumer reports whether a port direction consumes its signal (a sink).
func isConsumer(dir string) bool {
	switch dir {
	case "in", "inout":
		return true
	}
	return false
}

func sortedKeys(m map[string]bool) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}
