package generate

import (
	"os"
	"strings"
	"testing"

	"github.com/j-core/jcore-soc/tools/socgen/board"
	"github.com/j-core/jcore-soc/tools/socgen/elaborate"
)

func TestULX3SDefaultVariantReproducesJ2Binding(t *testing.T) {
	root := os.Getenv("JCORE_SOC_ROOT")
	if root == "" {
		t.Skip("JCORE_SOC_ROOT not set")
	}
	b, err := board.Load(root, "ulx3s", "") // default variant (j2-direct)
	if err != nil || b.Design == nil || b.Design.CPU == nil {
		t.Fatalf("load ulx3s: %v", err)
	}
	res, _ := elaborate.Elaborate(b)
	files, ferr := Build(b, res)
	if ferr != nil {
		t.Fatalf("Build: %v", ferr)
	}
	var cfg string
	for _, f := range files {
		if f.Name == "cpus_config.vhd" {
			cfg = f.Content
		}
	}
	// The default variant must bind the J2 direct synth config, matching the
	// retired hand-written one_cpu_m0_direct_fpga.
	if !strings.Contains(cfg, "for one_cpu_m0") ||
		!strings.Contains(cfg, "use configuration work.cpu_synth_direct;") {
		t.Errorf("default variant binding regressed:\n%s", cfg)
	}
	if strings.Contains(cfg, "generic map") {
		t.Errorf("J2 default must not bind PRIV_ARCH:\n%s", cfg)
	}
}
