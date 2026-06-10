package devicetree

import (
	"strconv"
	"strings"

	"github.com/j-core/jcore-soc/tools/socgen/iface"
	"github.com/j-core/jcore-soc/tools/socgen/vhdl"
)

// cpuFreq derives the CPU frequency (Hz) from the CFG_CLK_CPU_PERIOD_NS config
// constant (an integer literal, e.g. 20ns -> 50_000_000), faithful to
// device_tree.clj on-pregen (1e9 / period). Integer division (truncates for a
// period that does not divide 1e9 evenly — exact for all real J-core periods).
func cpuFreq(lib *iface.Library) (int, error) {
	period, ok := constInt(lib, "cfg_clk_cpu_period_ns")
	if !ok || period == 0 {
		return 0, &DTError{Kind: ErrCPUFreq}
	}
	return 1_000_000_000 / period, nil
}

// constInt finds a package constant by (case-insensitive) name and reads its
// integer-literal value.
func constInt(lib *iface.Library, name string) (int, bool) {
	if lib == nil {
		return 0, false
	}
	for _, pkg := range lib.Packages {
		for _, c := range pkg.Constants {
			if strings.EqualFold(c.Name, name) {
				// vhdl.INT excludes based literals (16#FF#); the lexer keeps the
				// raw text, so strip VHDL digit separators (50_000_000).
				if bl, ok := c.Value.(*vhdl.BasicLit); ok && bl.Kind == vhdl.INT {
					if n, err := strconv.Atoi(strings.ReplaceAll(bl.Value, "_", "")); err == nil {
						return n, true
					}
				}
			}
		}
	}
	return 0, false
}
