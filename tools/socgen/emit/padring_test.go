package emit

import (
	"strings"
	"testing"

	"github.com/j-core/jcore-soc/tools/socgen/design"
	"github.com/j-core/jcore-soc/tools/socgen/elaborate"
	"github.com/j-core/jcore-soc/tools/socgen/vhdl"
)

func TestPadRingPorts(t *testing.T) {
	res := &elaborate.Resolution{
		Pins: []*elaborate.ResolvedPin{
			{Net: "led0", Pad: "t18", PadDir: "out"},
			{Net: "clk_100mhz", Pad: "v10", PadDir: "in"},
			{Net: "mcb3_dram_dq0", Pad: "g3", PadDir: "inout"},
		},
	}
	ports := padRingPorts(res)
	got := map[string]string{}
	for _, p := range ports {
		got[p.Names[0]] = p.Mode
	}
	if got["pin_led0"] != "out" || got["pin_clk_100mhz"] != "in" || got["pin_mcb3_dram_dq0"] != "inout" {
		t.Errorf("pad ports = %v", got)
	}
	for _, p := range ports {
		if p.SubtypeMark != "std_logic" {
			t.Errorf("%s subtype = %q, want std_logic", p.Names[0], p.SubtypeMark)
		}
	}
}

func TestPinAttrs(t *testing.T) {
	res := &elaborate.Resolution{
		Pins: []*elaborate.ResolvedPin{
			{Net: "led0", Pad: "t18", PadDir: "out", Attrs: map[string]design.Value{
				"tig":        {Kind: design.KindExpr, Text: "yes"},
				"iostandard": {Kind: design.KindExpr, Text: "LVCMOS33"}, // buffer generic -> excluded
				"drive":      {Kind: design.KindInt, Int: 24},           // buffer generic -> excluded
			}},
		},
	}
	out := renderDecls2(t, pinAttrs(res))
	for _, want := range []string{
		"attribute loc : string;",
		`attribute loc of pin_led0 : signal is "t18";`,
		`attribute tig of pin_led0 : signal is "yes";`,
	} {
		if !contains3(out, want) {
			t.Errorf("pinAttrs missing %q:\n%s", want, out)
		}
	}
	for _, no := range []string{"iostandard", "drive"} {
		if contains3(out, no) {
			t.Errorf("buffer-generic %q must NOT be a pad attribute:\n%s", no, out)
		}
	}
}

// renderDecls2 prints decls inside a throwaway architecture.
func renderDecls2(t *testing.T, decls []vhdl.Decl) string {
	t.Helper()
	df := &vhdl.DesignFile{Units: []vhdl.DesignUnit{
		&vhdl.ArchitectureBody{Name: "impl", Entity: "pad_ring", Decls: decls},
	}}
	return vhdl.Print(df)
}
func contains3(s, sub string) bool { return strings.Contains(s, sub) }
