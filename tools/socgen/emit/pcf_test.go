package emit

import (
	"strings"
	"testing"

	"github.com/j-core/jcore-soc/tools/socgen/design"
	"github.com/j-core/jcore-soc/tools/socgen/elaborate"
)

func TestPCFEmitsSetIo(t *testing.T) {
	res := &elaborate.Resolution{
		Target: "ice40",
		Pins: []*elaborate.ResolvedPin{
			{Net: "clk", Pad: "35", In: "clk_in", PadDir: "in"},
			{Net: "ledr", Pad: "41", Out: "led_o0", PadDir: "out"},
			{Net: "nopad", Out: "x", PadDir: "out"}, // no Pad -> skipped
		},
	}
	out, err := PCF(res)
	if err != nil {
		t.Fatalf("PCF: %v", err)
	}
	for _, w := range []string{`set_io pin_clk 35`, `set_io pin_ledr 41`} {
		if !strings.Contains(out, w) {
			t.Errorf("pcf missing %q:\n%s", w, out)
		}
	}
	if strings.Contains(out, "pin_nopad") {
		t.Errorf("pin without a pad must be skipped:\n%s", out)
	}
	if strings.Contains(out, "IO_TYPE") || strings.Contains(out, "LOCATE") {
		t.Errorf("pcf must not contain LPF/IO_TYPE syntax:\n%s", out)
	}
}

// nextpnr-ice40's PCF has no io-standard directive: an LVDS-input pin must be
// emitted as a BARE set_io (the SB_LVDS_INPUT standard is applied via the
// SB_IO cell IO_STANDARD parameter in pad_ring, not the PCF). Emitting an
// `-io_std` token makes nextpnr treat the pin as unconstrained and abort, so
// guard against a regression that reintroduces it.
func TestPCFLVDSInputStaysBare(t *testing.T) {
	res := &elaborate.Resolution{
		Target: "ice40",
		Pins: []*elaborate.ResolvedPin{
			{Net: "mdi1_p", Pad: "42", In: "mdi1", PadDir: "in",
				Attrs: map[string]design.Value{
					"iostandard": {Kind: design.KindExpr, Text: "SB_LVDS_INPUT"}}},
		},
	}
	out, err := PCF(res)
	if err != nil {
		t.Fatalf("PCF: %v", err)
	}
	if !strings.Contains(out, "set_io pin_mdi1_p 42") {
		t.Errorf("LVDS-input pin must still be constrained to its pad:\n%s", out)
	}
	if strings.Contains(out, "io_std") || strings.Contains(out, "SB_LVDS_INPUT") {
		t.Errorf("PCF must NOT carry an io-standard token (nextpnr-ice40 rejects it):\n%s", out)
	}
}

func TestExternalConstraints(t *testing.T) {
	for _, tt := range []struct {
		target string
		want   bool
	}{{"ecp5", true}, {"ice40", true}, {"spartan6", false}, {"", false}} {
		if got := externalConstraints(tt.target); got != tt.want {
			t.Errorf("externalConstraints(%q)=%v want %v", tt.target, got, tt.want)
		}
	}
}
