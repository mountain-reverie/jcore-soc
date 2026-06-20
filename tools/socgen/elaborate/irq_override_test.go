package elaborate

import (
	"testing"

	"github.com/j-core/jcore-soc/tools/socgen/design"
	"github.com/j-core/jcore-soc/tools/socgen/iface"
)

func aicResForOverride() *Resolution {
	return &Resolution{
		Classes: map[string]*ResolvedClass{"aic": {Entity: &iface.Entity{Name: "aic"}}},
		Devices: []*ResolvedDevice{
			{Name: "aic0", Class: "aic", Ports: []*ResolvedPort{{Name: "irq_i", Kind: KindIRQ}}},
		},
	}
}

// An aic whose design maps irq_i explicitly must NOT have irq_i auto-wired to
// irqs<cpu> — the board owns its irq vector (e.g. a raw-pin IRQ source like the
// ULX3S button). Without this, the explicit mapping is silently discarded.
func TestResolveIRQExplicitIrqINotOverridden(t *testing.T) {
	d := &design.Design{Devices: []*design.Device{
		{Name: "aic0", Class: "aic", CPU: ip(0),
			Ports: map[string]design.Value{"irq_i": {Kind: design.KindExpr, Text: "aic_irq"}}},
	}}
	m := resolveIRQ(aicResForOverride(), d)
	if got := m.PortOverrides["aic0"]["irq_i"]; got != "" {
		t.Errorf("explicit irq_i must not be auto-overridden; got irqs override %q", got)
	}
}

// Control: without an explicit irq_i mapping, irq_i IS auto-wired to irqs0.
func TestResolveIRQDefaultIrqIAutoWired(t *testing.T) {
	d := &design.Design{Devices: []*design.Device{{Name: "aic0", Class: "aic", CPU: ip(0)}}}
	m := resolveIRQ(aicResForOverride(), d)
	if got := m.PortOverrides["aic0"]["irq_i"]; got != "irqs0" {
		t.Errorf("default irq_i should auto-wire to irqs0; got %q", got)
	}
}
