package emit

import (
	"os"
	"strings"
	"testing"

	"github.com/j-core/jcore-soc/tools/socgen/board"
	"github.com/j-core/jcore-soc/tools/socgen/elaborate"
)

func TestTopEntityGenericsMimasV2(t *testing.T) {
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
	soc, _ := SoC(res)

	// ddr_ctrl generic map is bool-free -> byte-exact after this milestone.
	ddr := "    ddr_ctrl : entity work.ddr_fsm(logic)\n" +
		"        generic map (\n" +
		"            read_sample_tm => freq_to_read_sample_tm(CFG_CLK_MEM_FREQ_HZ)\n" +
		"        )\n"
	if !strings.Contains(soc, ddr) {
		t.Errorf("soc.vhd ddr_ctrl generic map mismatch; got around:\n%s", around(soc, "ddr_ctrl : entity"))
	}

	// cpus generic map STRUCTURE (sorted formals; FALSE-casing is a later milestone).
	cpusBlock := around(soc, "cpus : ")
	for _, frag := range []string{
		"generic map (",
		"insert_inst_delay_boot_mem =>",
		"insert_read_delay_boot_mem =>",
		"insert_write_delay_boot_mem =>",
	} {
		if !strings.Contains(cpusBlock, frag) {
			t.Errorf("cpus generic map missing %q; got:\n%s", frag, cpusBlock)
		}
	}
	ii := strings.Index(cpusBlock, "insert_inst_delay")
	ir := strings.Index(cpusBlock, "insert_read_delay")
	iw := strings.Index(cpusBlock, "insert_write_delay")
	if !(ii >= 0 && ii < ir && ir < iw) {
		t.Errorf("cpus generics not in sorted order (inst<read<write):\n%s", cpusBlock)
	}

	// pad_ring padring-entity generics are emitted too (topInstStmt is shared):
	// ddr_clkgen's generic map is bool-free -> byte-exact.
	pad, _ := PadRing(res)
	ddrClk := "    ddr_clkgen : entity work.ddr_clkgen(interface)\n" +
		"        generic map (\n" +
		"            clk_i_period => CFG_CLK_CPU_PERIOD_NS\n" +
		"        )\n"
	if !strings.Contains(pad, ddrClk) {
		t.Errorf("pad_ring.vhd ddr_clkgen generic map mismatch; got around:\n%s", around(pad, "ddr_clkgen : entity"))
	}
}
