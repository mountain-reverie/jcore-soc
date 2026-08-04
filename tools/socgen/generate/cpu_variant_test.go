package generate

import (
	"os"
	"strings"
	"testing"

	"github.com/j-core/jcore-soc/tools/socgen/board"
	"github.com/j-core/jcore-soc/tools/socgen/elaborate"
)

// TestBuildEmitsCPUVariantForJ4Board loads the gf180_j4mmu board (a real j4
// board) and asserts its generated build.mk carries CPU_VARIANT := j4. This
// proves design.yaml's `model:` key reaches Make without Make ever parsing
// YAML.
func TestBuildEmitsCPUVariantForJ4Board(t *testing.T) {
	root := os.Getenv("JCORE_SOC_ROOT")
	if root == "" {
		t.Skip("JCORE_SOC_ROOT not set")
	}
	b, err := board.Load(root, "gf180_j4mmu", "")
	if err != nil || b == nil || b.Design == nil || b.Design.CPU == nil {
		t.Skip("gf180_j4mmu not available or not yet migrated to cpu: block")
	}
	res, _ := elaborate.Elaborate(b)
	files, err := Build(b, res)
	if err != nil {
		t.Logf("Build notes (tolerated in hg-less env): %v", err)
	}
	var buildMK string
	for _, f := range files {
		if f.Name == "build.mk" {
			buildMK = f.Content
		}
	}
	if !strings.Contains(buildMK, "CPU_VARIANT := j4") {
		t.Errorf("build.mk missing CPU_VARIANT := j4\ngot:\n%s", buildMK)
	}
}
