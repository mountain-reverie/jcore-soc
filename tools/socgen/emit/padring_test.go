package emit

import (
	"strings"
	"testing"

	"github.com/j-core/jcore-soc/tools/socgen/design"
	"github.com/j-core/jcore-soc/tools/socgen/elaborate"
	"github.com/j-core/jcore-soc/tools/socgen/iface"
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
	out := renderDeclsForTest(t, pinAttrs(res))
	for _, want := range []string{
		"attribute loc : string;",
		`attribute loc of pin_led0 : signal is "t18";`,
		`attribute tig of pin_led0 : signal is "yes";`,
	} {
		if !containsStr(out, want) {
			t.Errorf("pinAttrs missing %q:\n%s", want, out)
		}
	}
	for _, no := range []string{"iostandard", "drive"} {
		if containsStr(out, no) {
			t.Errorf("buffer-generic %q must NOT be a pad attribute:\n%s", no, out)
		}
	}
}

// renderDeclsForTest prints decls inside a throwaway architecture.
func renderDeclsForTest(t *testing.T, decls []vhdl.Decl) string {
	t.Helper()
	df := &vhdl.DesignFile{Units: []vhdl.DesignUnit{
		&vhdl.ArchitectureBody{Name: "impl", Entity: "pad_ring", Decls: decls},
	}}
	return vhdl.Print(df)
}
func containsStr(s, sub string) bool { return strings.Contains(s, sub) }

func TestPinAttrsLocLowercase(t *testing.T) {
	res := &elaborate.Resolution{
		Pins: []*elaborate.ResolvedPin{
			{Net: "clk_100mhz", Pad: "V10", PadDir: "in"},
		},
	}
	out := renderDeclsForTest(t, pinAttrs(res))
	want := `attribute loc of pin_clk_100mhz : signal is "v10";`
	if !containsStr(out, want) {
		t.Errorf("pinAttrs loc should be lowercased, missing %q:\n%s", want, out)
	}
	no := `"V10"`
	if containsStr(out, no) {
		t.Errorf("pinAttrs loc must NOT be uppercase %q:\n%s", no, out)
	}
}

func TestPadRingAssembly(t *testing.T) {
	res := &elaborate.Resolution{
		Pins: []*elaborate.ResolvedPin{
			{Net: "led0", Pad: "t18", PadDir: "out"},
		},
		PadringEntities: map[string]*elaborate.ResolvedEntity{
			"pll_250": {Name: "pll_250", Entity: &iface.Entity{Name: "pll_250"}, ArchName: "xilinx",
				Ports: []*elaborate.ResolvedPort{{Name: "rst", Kind: elaborate.KindSignal, GlobalSignal: "pll_rst"}}},
		},
		Signals: map[string]*elaborate.Signal{
			"clk_sys": {Name: "clk_sys", Type: &elaborate.ResolvedType{Mark: "std_logic"}},
			"pll_rst": {Name: "pll_rst", Type: &elaborate.ResolvedType{Mark: "std_logic"}},
		},
		SignalLocations: &elaborate.SignalLocations{
			PadringTop: []elaborate.PortLoc{{Name: "clk_sys", Dir: "in"}},
			Padring:    []string{"pll_rst"},
		},
	}
	out, err := PadRing(res)
	if err != nil {
		t.Fatalf("PadRing: %v", err)
	}
	if _, perr := vhdl.ParseFile(vhdl.NewFileSet(), "pad_ring.vhd", []byte(out)); perr != nil {
		t.Fatalf("re-parse: %v\n%s", perr, out)
	}
	for _, want := range []string{
		"entity pad_ring is",
		"pin_led0 : out std_logic",                     // pin port
		`attribute loc of pin_led0 : signal is "t18";`, // attr
		"signal pll_rst",                               // Padring internal decl
		"soc : entity work.soc(impl)",                  // soc instance
		"clk_sys => clk_sys",                           // soc port map wiring
		"pll_250 : entity work.pll_250(xilinx)",        // padring-entity instance
	} {
		if !strings.Contains(out, want) {
			t.Errorf("PadRing output missing %q:\n%s", want, out)
		}
	}
}
