package emit

import (
	"strings"
	"testing"

	"github.com/j-core/jcore-soc/tools/socgen/design"
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

func TestLPFPullmodeAttr(t *testing.T) {
	res := &elaborate.Resolution{
		Target: "ecp5",
		Pins: []*elaborate.ResolvedPin{
			{
				Net: "btn0", Pad: "D6", In: "gpio_di(0)", Signal: "ext_rst", PadDir: "in",
				Attrs: map[string]design.Value{
					"pullmode": {Kind: design.KindExpr, Text: "DOWN"},
				},
			},
		},
	}
	out, err := LPF(res)
	if err != nil {
		t.Fatalf("LPF: %v", err)
	}
	want := `IOBUF PORT "pin_btn0" IO_TYPE=LVCMOS33 PULLMODE=DOWN;`
	if !strings.Contains(out, want) {
		t.Errorf("btn0 LPF line missing PULLMODE=DOWN; got:\n%s", out)
	}
}

func TestLPFDriveAndSlewrateAttrs(t *testing.T) {
	res := &elaborate.Resolution{
		Target: "ecp5",
		Pins: []*elaborate.ResolvedPin{
			{
				Net: "sdram_d0", Pad: "J16", In: "sd_d(0)", PadDir: "in",
				Attrs: map[string]design.Value{
					"drive":    {Kind: design.KindInt, Int: 4},
					"slewrate": {Kind: design.KindExpr, Text: "FAST"},
				},
			},
		},
	}
	out, err := LPF(res)
	if err != nil {
		t.Fatalf("LPF: %v", err)
	}
	want := `IOBUF PORT "pin_sdram_d0" IO_TYPE=LVCMOS33 DRIVE=4 SLEWRATE=FAST;`
	if !strings.Contains(out, want) {
		t.Errorf("sdram_d0 LPF line missing DRIVE=4 SLEWRATE=FAST; got:\n%s", out)
	}
}
