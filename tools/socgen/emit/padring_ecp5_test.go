package emit

import (
	"testing"

	"github.com/j-core/jcore-soc/tools/socgen/design"
	"github.com/j-core/jcore-soc/tools/socgen/elaborate"
)

func TestPinAttrsSuppressedForEcp5(t *testing.T) {
	res := &elaborate.Resolution{
		Target: "ecp5",
		Pins: []*elaborate.ResolvedPin{
			{Net: "clk25", Pad: "G2", In: "clk_in", PadDir: "in",
				Attrs: map[string]design.Value{"iostandard": {Kind: design.KindStr, Text: "LVCMOS33"}}},
		},
	}
	if decls := pinAttrs(res); len(decls) != 0 {
		t.Errorf("pinAttrs for ecp5 = %d decls, want 0", len(decls))
	}
}
