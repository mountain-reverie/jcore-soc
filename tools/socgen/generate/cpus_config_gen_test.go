package generate

import (
	"os"
	"strings"
	"testing"

	"github.com/j-core/jcore-soc/tools/socgen/board"
	"github.com/j-core/jcore-soc/tools/socgen/elaborate"
)

func TestBuildEmitsCPUsConfig(t *testing.T) {
	root := os.Getenv("JCORE_SOC_ROOT")
	if root == "" {
		t.Skip("JCORE_SOC_ROOT not set")
	}
	b, _ := board.Load(root, "ulx3s", "")
	if b == nil || b.Design == nil || b.Design.CPU == nil {
		t.Skip("ulx3s not yet migrated to cpu: block")
	}
	res, _ := elaborate.Elaborate(b)
	files, err := Build(b, res)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	var got *File
	for i := range files {
		if files[i].Name == "cpus_config.vhd" {
			got = &files[i]
		}
	}
	if got == nil {
		t.Fatal("cpus_config.vhd not emitted")
	}
	if !strings.Contains(got.Content, "configuration soc_cpus_config of cpus is") {
		t.Errorf("cpus_config.vhd content wrong:\n%s", got.Content)
	}
}
