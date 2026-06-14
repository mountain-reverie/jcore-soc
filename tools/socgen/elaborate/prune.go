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
//
// DIVERGENCE (microboard + turtle): the aic outputs rtc_sec/rtc_nsec are write-only on
// every board (the aic drives them; no device reads them — emac's rtc_sec_i inputs are
// tied to constants). mimas's golden prunes them (we match); microboard's and
// turtle_1v0's *committed* goldens instead declare `signal rtc_sec/rtc_nsec` and wire
// them — a stale artifact on both (same class as the pad_ring clock_locked1 divergence,
// P6b-3f / MB-3), almost certainly predating this prune. We deliberately keep the
// consistent prune across all boards (the cleaner netlist: the dead aic RTC logic is
// synthesised away either way, and `=> open` is the canonical unused-output idiom). So
// microboard and turtle devices.vhd intentionally lack those two signal declarations vs
// their respective goldens. See TestDevicesMicroboardRtcDivergence and
// TestDevicesTurtleRtcDivergence.
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
	// Clear references to a pruned signal on every entity port so emit renders the
	// port `open`. This mirrors Clojure's global signal-map lookup (instantiate-ports
	// does `(get signals global-signal)` for device, top AND padring entities, so a
	// pruned signal becomes nil → open everywhere). mimas_v2's write-only signals are
	// all device-context, but covering top/padring too avoids a dangling reference in
	// soc.vhd/pad_ring.vhd on a board where one originates there.
	for _, dev := range res.Devices {
		clearPrunedRefs(dev.Ports, pruned)
	}
	for _, e := range res.TopEntities {
		clearPrunedRefs(e.Ports, pruned)
	}
	for _, e := range res.PadringEntities {
		clearPrunedRefs(e.Ports, pruned)
	}
}

// clearPrunedRefs clears GlobalSignal on each KindSignal port that references a
// pruned signal, so emit renders the port actual as `open`.
func clearPrunedRefs(ports []*ResolvedPort, pruned map[string]bool) {
	for _, p := range ports {
		if p.Kind == KindSignal && pruned[p.GlobalSignal] {
			p.GlobalSignal = ""
		}
	}
}
