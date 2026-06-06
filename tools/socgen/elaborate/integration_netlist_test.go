package elaborate

import (
	"os"
	"testing"

	"github.com/j-core/jcore-soc/tools/socgen/board"
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
	res, errs := Elaborate(b)
	t.Logf("mimas_v2 net-list: %d devices, %d signals, %d errors", len(res.Devices), len(res.Signals), len(errs))
	// Validation errors are INFORMATIONAL here: this is a device-only net-list, so
	// signals driven only by an unresolved top/padring port show as "nothing drives".
	// Log + count them; do NOT fail on them.
	for _, e := range errs {
		t.Logf("  %v", e)
	}
	if len(res.Signals) == 0 {
		t.Error("expected a non-empty net-list")
	}
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
