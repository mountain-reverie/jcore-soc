package elaborate

import (
	"errors"
	"testing"

	"github.com/j-core/jcore-soc/tools/socgen/design"
	"github.com/j-core/jcore-soc/tools/socgen/iface"
)

func ip(i int) *int { return &i }

func TestResolveIRQSimple(t *testing.T) {
	i4 := 4
	res := &Resolution{
		Classes: map[string]*ResolvedClass{
			"aic":  {Entity: &iface.Entity{Name: "aic"}},
			"gpio": {Entity: &iface.Entity{Name: "pio"}},
		},
		Devices: []*ResolvedDevice{
			{Name: "aic0", Class: "aic", Ports: []*ResolvedPort{{Name: "irq_i", Kind: KindIRQ}}},
			{Name: "gpio", Class: "gpio", Ports: []*ResolvedPort{{Name: "irq", Kind: KindIRQ}}},
		},
	}
	d := &design.Design{Devices: []*design.Device{
		{Name: "aic0", Class: "aic", CPU: ip(0)},
		{Name: "gpio", Class: "gpio", IRQ: &design.IRQRef{Int: &i4}},
	}}
	m := resolveIRQ(res, d)
	if m == nil {
		t.Fatal("nil IRQ model")
	}
	if m.PortOverrides["gpio"]["irq"] != "irqs0(4)" {
		t.Errorf("gpio.irq = %q, want irqs0(4)", m.PortOverrides["gpio"]["irq"])
	}
	if m.PortOverrides["aic0"]["irq_i"] != "irqs0" {
		t.Errorf("aic0.irq_i = %q, want irqs0", m.PortOverrides["aic0"]["irq_i"])
	}
	if m.VectorNumbers["aic0"][4] != 0x15 {
		t.Errorf("vector[4] = %#x, want 0x15", m.VectorNumbers["aic0"][4])
	}
	if len(m.Signals) == 0 || m.Signals[0].Name != "irqs0" || m.Signals[0].Width != 8 {
		t.Errorf("signals[0] = %+v, want irqs0 width 8", m.Signals)
	}
}

// A device whose irq targets a cpu that HAS NO aic resolves to "open" (golden
// cache.int1 => open, cpu1 has no aic). An aic on cpu0 makes the model non-nil;
// the cache.int1 entry on cpu1 exercises the open case.
func TestResolveIRQNoAicOpen(t *testing.T) {
	res := &Resolution{
		Classes: map[string]*ResolvedClass{
			"aic":   {Entity: &iface.Entity{Name: "aic"}},
			"cache": {Entity: &iface.Entity{Name: "icache_modereg"}},
		},
		Devices: []*ResolvedDevice{
			{Name: "aic0", Class: "aic", Ports: []*ResolvedPort{{Name: "irq_i", Kind: KindIRQ}}},
			{Name: "cache", Class: "cache", Ports: []*ResolvedPort{{Name: "int1", Kind: KindIRQ}}},
		},
	}
	d := &design.Design{Devices: []*design.Device{
		{Name: "aic0", Class: "aic", CPU: ip(0)},
		{Name: "cache", Class: "cache", IRQ: &design.IRQRef{Named: map[string]*design.IRQEntry{"int1": {CPU: 1, IRQ: 3}}}},
	}}
	m := resolveIRQ(res, d)
	if m == nil {
		t.Fatal("nil IRQ model (an aic on cpu0 exists)")
	}
	if m.PortOverrides["cache"]["int1"] != "open" {
		t.Errorf("cache.int1 (no aic on cpu1) = %q, want open", m.PortOverrides["cache"]["int1"])
	}
}

// An irq number outside 0..7 (path 8) is a validation error: buildIRQ must
// return a non-empty error slice carrying ErrIRQBadPath.
func TestResolveIRQValidation(t *testing.T) {
	i8 := 8
	res := &Resolution{
		Classes: map[string]*ResolvedClass{
			"aic":  {Entity: &iface.Entity{Name: "aic"}},
			"gpio": {Entity: &iface.Entity{Name: "pio"}},
		},
		Devices: []*ResolvedDevice{
			{Name: "aic0", Class: "aic", Ports: []*ResolvedPort{{Name: "irq_i", Kind: KindIRQ}}},
			{Name: "gpio", Class: "gpio", Ports: []*ResolvedPort{{Name: "irq", Kind: KindIRQ}}},
		},
	}
	d := &design.Design{Devices: []*design.Device{
		{Name: "aic0", Class: "aic", CPU: ip(0)},
		{Name: "gpio", Class: "gpio", IRQ: &design.IRQRef{Int: &i8}},
	}}
	_, errs := buildIRQ(res, d)
	if len(errs) == 0 {
		t.Fatal("buildIRQ returned no errors for out-of-range irq path 8")
	}
	found := false
	for _, err := range errs {
		if errors.Is(err, ErrIRQBadPath) {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("errors %v, want one matching ErrIRQBadPath", errs)
	}
}

func TestResolveIRQOrCombine(t *testing.T) {
	i5a, i5b := 5, 5
	res := &Resolution{
		Classes: map[string]*ResolvedClass{
			"aic": {Entity: &iface.Entity{Name: "aic"}},
			"d":   {Entity: &iface.Entity{Name: "d"}},
		},
		Devices: []*ResolvedDevice{
			{Name: "aic0", Class: "aic", Ports: []*ResolvedPort{{Name: "irq_i", Kind: KindIRQ}}},
			{Name: "da", Class: "d", Ports: []*ResolvedPort{{Name: "irq", Kind: KindIRQ}}},
			{Name: "db", Class: "d", Ports: []*ResolvedPort{{Name: "irq", Kind: KindIRQ}}},
		},
	}
	d := &design.Design{Devices: []*design.Device{
		{Name: "aic0", Class: "aic", CPU: ip(0)},
		{Name: "da", Class: "d", IRQ: &design.IRQRef{Int: &i5a}},
		{Name: "db", Class: "d", IRQ: &design.IRQRef{Int: &i5b}},
	}}
	m := resolveIRQ(res, d)
	if m.PortOverrides["da"]["irq"] != "irq_0_5_a" || m.PortOverrides["db"]["irq"] != "irq_0_5_b" {
		t.Errorf("OR sources wrong: da=%q db=%q", m.PortOverrides["da"]["irq"], m.PortOverrides["db"]["irq"])
	}
	if len(m.OrAssigns) != 1 || m.OrAssigns[0].Target != "irqs0(5)" || len(m.OrAssigns[0].Sources) != 2 {
		t.Fatalf("OrAssigns = %+v, want one targeting irqs0(5) with 2 sources", m.OrAssigns)
	}
}
