package emit

import (
	"os"
	"strings"
	"testing"

	"github.com/j-core/jcore-soc/tools/socgen/board"
	"github.com/j-core/jcore-soc/tools/socgen/elaborate"
)

func TestPackagesScoped(t *testing.T) {
	lib := libFrom(t, `package data_bus_pack is
  type data_bus_i_t is record a : std_logic; end record;
end package;
package cpu2j0_pack is
  type cpu_data_i_t is record b : std_logic; end record;
end package;`)
	// Marks include duplicates, a std type (skipped), and two work types.
	// All are single-package, so owners are irrelevant (nil).
	got := packagesScoped(lib, []string{"cpu_data_i_t", "std_logic", "data_bus_i_t", "cpu_data_i_t"}, nil)
	want := []string{"cpu2j0_pack", "data_bus_pack"} // distinct + sorted
	if len(got) != len(want) {
		t.Fatalf("packagesScoped = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("packagesScoped[%d] = %q, want %q (full %v)", i, got[i], want[i], got)
		}
	}
	// nil lib -> empty, no panic.
	if p := packagesScoped(nil, []string{"cpu_data_i_t"}, nil); len(p) != 0 {
		t.Errorf("packagesScoped(nil) = %v, want empty", p)
	}
}

func TestMergeSortedPkg(t *testing.T) {
	if got := mergeSortedPkg([]string{"cpu2j0_pack", "ddrc_cnt_pack"}, "data_bus_pack"); len(got) != 3 ||
		got[0] != "cpu2j0_pack" || got[1] != "data_bus_pack" || got[2] != "ddrc_cnt_pack" {
		t.Errorf("mergeSortedPkg insert = %v, want [cpu2j0_pack data_bus_pack ddrc_cnt_pack]", got)
	}
	if got := mergeSortedPkg([]string{"data_bus_pack"}, "data_bus_pack"); len(got) != 1 {
		t.Errorf("mergeSortedPkg dup = %v, want [data_bus_pack]", got)
	}
}

// contextLines returns the leading `library`/`use` lines of an emitted VHDL
// file (the context block precedes the entity), trimmed, in order.
func contextLines(vhdlText string) []string {
	var out []string
	for _, ln := range strings.Split(vhdlText, "\n") {
		t := strings.TrimSpace(ln)
		switch {
		case strings.HasPrefix(t, "library ") || strings.HasPrefix(t, "use "):
			out = append(out, t)
		case t == "" || strings.HasPrefix(t, "--"):
			continue
		default:
			return out // first non-context line ends the block
		}
	}
	return out
}

func TestContextMimasV2(t *testing.T) {
	root := os.Getenv("JCORE_SOC_ROOT")
	if root == "" {
		t.Skip("JCORE_SOC_ROOT not set")
	}
	b, lerr := board.Load(root, "mimas_v2")
	if b == nil || b.Design == nil {
		t.Fatalf("board.Load: %v", lerr)
	}
	res, rerr := elaborate.Elaborate(b)
	if rerr != nil {
		t.Logf("elaborate notes: %v", rerr)
	}
	devVHD, derr := Devices(res)
	socVHD, serr := SoC(res)
	padVHD, perr := PadRing(res)
	if derr != nil || serr != nil || perr != nil {
		t.Logf("emit notes: devices=%v soc=%v padring=%v", derr, serr, perr)
	}

	wantDev := []string{
		"library ieee;", "use ieee.std_logic_1164.all;", "use ieee.numeric_std.all;",
		"use work.config.all;", "use work.clk_config.all;",
		"use work.cpu2j0_pack.all;", "use work.data_bus_pack.all;", "use work.ddrc_cnt_pack.all;",
	}
	// dma_pack sorts last among soc's work uses (alphabetical).
	wantSoc := append(append([]string{}, wantDev...), "use work.dma_pack.all;")
	wantPad := append(append([]string{}, wantSoc...), "library unisim;", "use unisim.vcomponents.all;")

	check := func(name string, got, want []string) {
		t.Helper()
		if len(got) != len(want) {
			t.Errorf("%s: %d context lines, want %d\n got %v\nwant %v", name, len(got), len(want), got, want)
			return
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("%s context[%d] = %q, want %q", name, i, got[i], want[i])
			}
		}
	}
	check("devices.vhd", contextLines(devVHD), wantDev)
	check("soc.vhd", contextLines(socVHD), wantSoc)
	check("pad_ring.vhd", contextLines(padVHD), wantPad)
}
