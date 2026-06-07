package elaborate

import (
	"errors"
	"os"
	"testing"

	"github.com/j-core/jcore-soc/tools/socgen/board"
	"github.com/j-core/jcore-soc/tools/socgen/internal/errutil"
)

func TestElaborateNetlistMimasV2(t *testing.T) {
	root := os.Getenv("JCORE_SOC_ROOT")
	if root == "" {
		t.Skip("JCORE_SOC_ROOT not set")
	}
	b, _ := board.Load(root, "mimas_v2")
	if b.Design == nil || b.Library == nil {
		t.Skip("board.Load incomplete")
	}
	res, err := Elaborate(b)
	errs := errutil.Errors(err)
	t.Logf("mimas_v2 net-list: %d devices, %d signals, %d errors", len(res.Devices), len(res.Signals), len(errs))
	// The net-list now spans devices + top/padring entities (P4d). Remaining
	// validation errors are INFORMATIONAL: a top/padring entity whose entity
	// could not be bound contributes no ports, and pin-driven signals (P4d-ii)
	// have no net-list driver yet — both legitimately show as "nothing drives".
	// Log + count them; do NOT fail on them.
	for _, e := range errs {
		t.Logf("  %v", e)
	}
	if len(res.Signals) == 0 {
		t.Error("expected a non-empty net-list")
	}
	// P4d: top/padring entities must resolve and contribute drivers.
	if len(res.TopEntities) == 0 {
		t.Error("expected resolved top-entities")
	}
	if len(res.PadringEntities) == 0 {
		t.Error("expected resolved padring-entities")
	}
	for name, re := range res.TopEntities {
		if re.Entity == nil {
			t.Errorf("top-entity %q did not bind an entity", name)
		}
	}
	for name, re := range res.PadringEntities {
		if re.Entity == nil {
			t.Errorf("padring-entity %q did not bind an entity", name)
		}
	}
	// the net-list now spans devices + top + padring: at least one signal must have
	// a top or padring port reference.
	spansTopPadring := false
	for _, s := range res.Signals {
		for _, pr := range s.Ports {
			if pr.Context.Kind == "top" || pr.Context.Kind == "padring" {
				spansTopPadring = true
			}
		}
	}
	if !spansTopPadring {
		t.Error("net-list does not span any top/padring port")
	}
	// The actual, provable value of P4d: a top/padring output now drives a device
	// input on a shared signal. (We do NOT assert the total "nothing drives" count
	// drops — it legitimately RISES, because the large top/padring entities — the
	// CPU complex, the DDR controller/mux, the pad iocells — bring their own many
	// consumed inputs that are pin-driven (P4d-ii) or inter-entity-wired, neither
	// modeled yet. The count is logged for visibility only.)
	joinDrivesDevice := false
	for _, s := range res.Signals {
		hasTopPadOut, hasDeviceIn := false, false
		for _, pr := range s.Ports {
			if (pr.Context.Kind == "top" || pr.Context.Kind == "padring") && pr.Dir == "out" {
				hasTopPadOut = true
			}
			if pr.Context.Kind == "device" && pr.Dir == "in" {
				hasDeviceIn = true
			}
		}
		if hasTopPadOut && hasDeviceIn {
			joinDrivesDevice = true
			break
		}
	}
	if !joinDrivesDevice {
		t.Error("expected the join to connect a top/padring output to a device input")
	}
	undriven := 0
	for _, e := range errs {
		if errors.Is(e, ErrUndrivenSignal) {
			undriven++
		}
	}
	t.Logf("mimas_v2 undriven signals: %d (informational; device-only baseline was 24, rises after the join — pins are P4d-ii)", undriven)
	// the real generic->type case: flash (spi2) cs : out std_logic_vector(num_cs-1 downto 0), num_cs=2 -> (1 downto 0)
	found := false
	for _, dev := range res.Devices {
		for _, p := range dev.Ports {
			if dev.Name == "flash" && p.Name == "cs" {
				found = true
				if p.Type.Left == nil || *p.Type.Left != 1 || p.Type.Dir != "downto" {
					t.Errorf("flash.cs type not resolved to (1 downto 0): %s", p.Type.String())
				}
			}
		}
	}
	if !found {
		t.Error("flash.cs port not found in net-list")
	}
}
