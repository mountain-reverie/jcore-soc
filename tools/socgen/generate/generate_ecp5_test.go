package generate

import (
	"testing"

	"github.com/j-core/jcore-soc/tools/socgen/board"
	"github.com/j-core/jcore-soc/tools/socgen/design"
	"github.com/j-core/jcore-soc/tools/socgen/elaborate"
)

func TestBuildEmitsLpfForEcp5(t *testing.T) {
	b := &board.Board{Name: "ecp5fixture", Design: &design.Design{Target: "ecp5"}}
	res := &elaborate.Resolution{
		Target: "ecp5",
		Pins: []*elaborate.ResolvedPin{
			{Net: "led0", Pad: "B2", Out: "led_o0", PadDir: "out"},
		},
	}
	files, _ := Build(b, res) // core emits may warn on a minimal res; we only assert the .lpf
	var found bool
	for _, f := range files {
		if f.Name == "ecp5fixture.lpf" {
			found = true
			if f.InBuildMK {
				t.Errorf(".lpf must not be listed in build.mk")
			}
		}
	}
	if !found {
		t.Errorf("Build did not emit ecp5fixture.lpf; files = %v", files)
	}
}
