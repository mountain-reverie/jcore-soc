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

func TestRemoveWriteOnlySignals(t *testing.T) {
	woPort := &ResolvedPort{Name: "busy", Kind: KindSignal, GlobalSignal: "flash_busy"}
	res := &Resolution{
		Devices: []*ResolvedDevice{{Name: "flash", Ports: []*ResolvedPort{woPort}}},
		Signals: map[string]*Signal{
			// write-only: a single out port, no reader -> pruned.
			"flash_busy": {Name: "flash_busy", Ports: []*SignalPortRef{
				{Context: Context{Kind: "device", ID: "flash"}, PortName: "busy", Dir: dirOut},
			}},
			// has a reader -> kept.
			"clk_sys": {Name: "clk_sys", Ports: []*SignalPortRef{
				{Context: Context{Kind: "device", ID: "flash"}, PortName: "clk", Dir: dirIn},
			}},
			// pin context (out dir) -> kept.
			"led0": {Name: "led0", Ports: []*SignalPortRef{
				{Context: Context{Kind: ctxKindPin, ID: "led0"}, PortName: "led0", Dir: dirOut},
			}},
			// out-only but in ZeroSignals -> kept.
			"zeroed": {Name: "zeroed", Ports: []*SignalPortRef{
				{Context: Context{Kind: "device", ID: "x"}, PortName: "z", Dir: dirOut},
			}},
		},
	}
	d := &design.Design{ZeroSignals: []string{"zeroed"}}
	removeWriteOnlySignals(res, d)
	if _, ok := res.Signals["flash_busy"]; ok {
		t.Errorf("flash_busy (write-only) should be pruned")
	}
	for _, keep := range []string{"clk_sys", "led0", "zeroed"} {
		if _, ok := res.Signals[keep]; !ok {
			t.Errorf("%s should be kept", keep)
		}
	}
	if woPort.GlobalSignal != "" {
		t.Errorf("pruned signal's port should have GlobalSignal cleared, got %q", woPort.GlobalSignal)
	}
}

func TestRemoveWriteOnlySignalsClearsEntityPorts(t *testing.T) {
	// A top/padring entity port referencing a pruned write-only signal must also
	// have GlobalSignal cleared (Clojure's signal-map miss → open is global).
	topPort := &ResolvedPort{Name: "dbg", Kind: KindSignal, GlobalSignal: "wo"}
	padPort := &ResolvedPort{Name: "dbg", Kind: KindSignal, GlobalSignal: "wo"}
	res := &Resolution{
		TopEntities:     map[string]*ResolvedEntity{"t": {Name: "t", Ports: []*ResolvedPort{topPort}}},
		PadringEntities: map[string]*ResolvedEntity{"p": {Name: "p", Ports: []*ResolvedPort{padPort}}},
		Signals: map[string]*Signal{
			"wo": {Name: "wo", Ports: []*SignalPortRef{
				{Context: Context{Kind: "device", ID: "x"}, PortName: "o", Dir: dirOut},
			}},
		},
	}
	removeWriteOnlySignals(res, &design.Design{})
	if topPort.GlobalSignal != "" || padPort.GlobalSignal != "" {
		t.Errorf("top/padring entity ports referencing a pruned signal should be cleared: top=%q pad=%q",
			topPort.GlobalSignal, padPort.GlobalSignal)
	}
}
