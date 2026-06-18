package emit

import (
	"strings"
	"testing"

	"github.com/j-core/jcore-soc/tools/socgen/elaborate"
)

func TestLPFEmitsLocateAndIobuf(t *testing.T) {
	res := &elaborate.Resolution{
		Target: "ecp5",
		Pins: []*elaborate.ResolvedPin{
			{Net: "clk25", Pad: "G2", In: "clk_in", PadDir: "in"},
			{Net: "led0", Pad: "B2", Out: "led_o0", PadDir: "out"},
			{Net: "nopad", Out: "x", PadDir: "out"}, // no Pad -> skipped
		},
	}
	out, err := LPF(res)
	if err != nil {
		t.Fatalf("LPF: %v", err)
	}
	want := []string{
		`LOCATE COMP "pin_clk25" SITE "G2";`,
		`IOBUF PORT "pin_clk25" IO_TYPE=LVCMOS33;`,
		`LOCATE COMP "pin_led0" SITE "B2";`,
	}
	for _, w := range want {
		if !strings.Contains(out, w) {
			t.Errorf("lpf missing %q:\n%s", w, out)
		}
	}
	if strings.Contains(out, "pin_nopad") {
		t.Errorf("pin without a pad must be skipped:\n%s", out)
	}
}
