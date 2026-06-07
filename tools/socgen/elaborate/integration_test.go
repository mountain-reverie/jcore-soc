package elaborate

import (
	"os"
	"testing"

	"github.com/j-core/jcore-soc/tools/socgen/board"
)

func TestElaborateMimasV2(t *testing.T) {
	root := os.Getenv("JCORE_SOC_ROOT")
	if root == "" {
		t.Skip("JCORE_SOC_ROOT not set")
	}
	b, lerrs := board.Load(root, "mimas_v2")
	if b.Design == nil || b.Library == nil {
		t.Fatalf("board.Load returned incomplete board: %v", lerrs)
	}
	res, errs := Devices(b)
	t.Logf("mimas_v2 elaborate: %d classes, %d devices, %d resolution errors", len(res.Classes), len(res.Devices), len(errs))
	for _, e := range errs {
		t.Logf("  %v", e)
	}
	if len(errs) != 0 {
		t.Errorf("want 0 resolution errors for mimas_v2, got %d", len(errs))
	}
	// every device resolves to a class with a bound entity + an architecture.
	for _, dev := range res.Devices {
		rc := res.Classes[lc(dev.Class)]
		if rc == nil || rc.Entity == nil {
			t.Errorf("device %q (class %q) did not resolve to an entity", dev.Name, dev.Class)
			continue
		}
		if rc.ArchName == "" && rc.Config == nil {
			t.Errorf("device %q (class %q) has no architecture/configuration", dev.Name, dev.Class)
		}
	}
	if len(res.Devices) == 0 {
		t.Error("expected resolved devices for mimas_v2")
	}
}
