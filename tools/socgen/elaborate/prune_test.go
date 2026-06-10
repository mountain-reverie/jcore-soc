package elaborate

import (
	"testing"

	"github.com/j-core/jcore-soc/tools/socgen/design"
)

func TestMarkNamedIRQPorts(t *testing.T) {
	ports := []*ResolvedPort{
		{Name: "int0", Kind: KindSignal, GlobalSignal: "cache_ctrl_int0"},
		{Name: "int1", Kind: KindSignal, GlobalSignal: "cache_ctrl_int1"},
		{Name: "clk", Kind: KindSignal, GlobalSignal: "clk_sys"},
	}
	irq := &design.IRQRef{Named: map[string]*design.IRQEntry{"int0": {}, "int1": {}}}
	markNamedIRQPorts(ports, irq)
	for _, p := range ports[:2] {
		if p.Kind != KindIRQ || p.GlobalSignal != "" {
			t.Errorf("%s: Kind=%v GlobalSignal=%q; want KindIRQ, \"\"", p.Name, p.Kind, p.GlobalSignal)
		}
	}
	if ports[2].Kind != KindSignal || ports[2].GlobalSignal != "clk_sys" {
		t.Errorf("clk should be untouched: %+v", ports[2])
	}
	// nil irq -> no panic, no change.
	markNamedIRQPorts(ports, nil)
}
