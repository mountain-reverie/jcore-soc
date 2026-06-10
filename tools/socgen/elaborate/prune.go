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
