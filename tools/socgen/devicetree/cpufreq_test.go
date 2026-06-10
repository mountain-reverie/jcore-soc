package devicetree

import (
	"testing"

	"github.com/j-core/jcore-soc/tools/socgen/iface"
	"github.com/j-core/jcore-soc/tools/socgen/vhdl"
)

func TestCPUFreq(t *testing.T) {
	lib := &iface.Library{Packages: map[string]*iface.Package{
		"config": {Name: "config", Constants: []*iface.Constant{
			{Name: "CFG_CLK_CPU_PERIOD_NS", Value: &vhdl.BasicLit{Kind: vhdl.INT, Value: "20"}},
		}},
	}}
	f, err := cpuFreq(lib)
	if err != nil {
		t.Fatalf("cpuFreq: %v", err)
	}
	if f != 50_000_000 {
		t.Errorf("cpuFreq = %d, want 50000000", f)
	}
}

func TestCPUFreqUnderscoreLiteral(t *testing.T) {
	// a VHDL int literal with digit separators must still parse.
	lib := &iface.Library{Packages: map[string]*iface.Package{
		"config": {Name: "config", Constants: []*iface.Constant{
			{Name: "CFG_CLK_CPU_PERIOD_NS", Value: &vhdl.BasicLit{Kind: vhdl.INT, Value: "1_000"}},
		}},
	}}
	if f, err := cpuFreq(lib); err != nil || f != 1_000_000 {
		t.Errorf("cpuFreq(period 1_000) = %d, %v; want 1000000, nil", f, err)
	}
}

func TestCPUFreqMissing(t *testing.T) {
	if _, err := cpuFreq(&iface.Library{Packages: map[string]*iface.Package{}}); err == nil {
		t.Errorf("want error for missing constant")
	}
}
