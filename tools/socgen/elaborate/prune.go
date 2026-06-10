package elaborate

import "github.com/j-core/jcore-soc/tools/socgen/design"

// markNamedIRQPorts marks each port whose name is a key in the device's
// instance-level irq map as an interrupt port (faithful to Clojure
// assign-device-ports, devices.clj:281: a port named in (:irq dev) is "special"
// and not connected by global-signal name). The IRQ model wires its actual (P5e);
// clearing GlobalSignal keeps the port out of the gathered net-list, so no orphan
// signal is declared for it.
func markNamedIRQPorts(ports []*ResolvedPort, irq *design.IRQRef) {
	if irq == nil || len(irq.Named) == 0 {
		return
	}
	for _, p := range ports {
		if p.Kind != KindSignal {
			continue
		}
		if _, named := irq.Named[p.Name]; named {
			p.Kind = KindIRQ
			p.GlobalSignal = ""
		}
	}
}

// removeWriteOnlySignals drops global signals that nothing reads, faithful to
// Clojure remove-write-only-signals (devices.clj:789-797): a signal is KEPT iff
// some port reads it (Dir == "in") OR some port has a pin context; otherwise it is
// a write-only signal (e.g. an unconnected device output) and is removed. Signals
// in ZeroSignals are always kept (they carry a synthetic driver). For each pruned
// signal, the referencing device ports have GlobalSignal cleared so emit renders
// the port actual as `open` (Clojure relies on the signal-map miss returning nil →
// open; clearing the port is the equivalent Go effect).
func removeWriteOnlySignals(res *Resolution, d *design.Design) {
	zero := make(map[string]bool, len(d.ZeroSignals))
	for _, z := range d.ZeroSignals {
		zero[z] = true
	}
	pruned := map[string]bool{}
	for name, sig := range res.Signals {
		if zero[name] {
			continue
		}
		read := false
		for _, pr := range sig.Ports {
			if pr.Dir == dirIn || pr.Context.Kind == ctxKindPin {
				read = true
				break
			}
		}
		if !read {
			delete(res.Signals, name)
			pruned[name] = true
		}
	}
	if len(pruned) == 0 {
		return
	}
	// Clear the cleared signal's references on device ports so emit renders them
	// `open`. Scoped to devices (as in Clojure): top/padring entity ports carry
	// their own GlobalSignal but a pruned write-only signal is device-context.
	for _, dev := range res.Devices {
		for _, p := range dev.Ports {
			if p.Kind == KindSignal && pruned[p.GlobalSignal] {
				p.GlobalSignal = ""
			}
		}
	}
}
