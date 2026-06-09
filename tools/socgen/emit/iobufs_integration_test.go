package emit

import (
	"os"
	"strings"
	"testing"

	"github.com/j-core/jcore-soc/tools/socgen/board"
	"github.com/j-core/jcore-soc/tools/socgen/elaborate"
	"github.com/j-core/jcore-soc/tools/socgen/vhdl"
)

func TestPadRingBuffersMimasV2(t *testing.T) {
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
	out, derr := PadRing(res)
	if derr != nil {
		t.Logf("emit notes: %v", derr)
	}
	if _, perr := vhdl.ParseFile(vhdl.NewFileSet(), "pad_ring.vhd", []byte(out)); perr != nil {
		t.Fatalf("emitted pad_ring.vhd does not re-parse: %v\n%s", perr, out)
	}
	// normalize whitespace for robust substring checks (printer spacing-agnostic).
	n := strings.Join(strings.Fields(out), " ")
	norm := func(s string) string { return strings.Join(strings.Fields(s), " ") }

	wants := []string{
		"library unisim;",
		"use unisim.vcomponents.all;",
		"clk_100mhz <= pin_clk_100mhz;", // direct-wire (buff:false); printer emits no space before ';'
		"obuf_led0 : OBUF",
		"I => po(0)", "O => pin_led0", "DRIVE => 24", `IOSTANDARD => "LVCMOS33"`,
		"ibuf_sd_miso : IBUF", "I => pin_sd_miso", "O => sd_miso",
		"iobuf_mcb3_dram_dq0 : IOBUF",
		"I => dr_data_o.dqo(0)", "T => dr_data_o.dq_outen(0)", "O => dr_data_i.dqi(0)", "IO => pin_mcb3_dram_dq0",
		"obuft_mcb3_dram_ldm : OBUFT", "I => dr_data_o.dmo(0)", "T => dr_data_o.dq_outen(16)", "O => pin_mcb3_dram_ldm",
		"obufds_mcb3_dram_ck_mcb3_dram_ck_n : OBUFDS",
		"I => ddr_clk", "O => pin_mcb3_dram_ck", "OB => pin_mcb3_dram_ck_n",
	}
	for _, w := range wants {
		if !strings.Contains(n, norm(w)) {
			t.Errorf("pad_ring buffers missing %q", w)
		}
	}
	// IBUF must NOT carry DRIVE/SLEW generics (create-ibuf dissoc).
	if i := strings.Index(n, "ibuf_sd_miso : IBUF"); i >= 0 {
		seg := n[i:]
		if j := strings.Index(seg, "port map"); j >= 0 {
			if head := seg[:j]; strings.Contains(head, "DRIVE") || strings.Contains(head, "SLEW") {
				t.Errorf("ibuf_sd_miso must not have DRIVE/SLEW generics:\n%s", head)
			}
		}
	}
	count := strings.Count(n, " : OBUF ") + strings.Count(n, " : IBUF ") + strings.Count(n, " : IOBUF ") + strings.Count(n, " : OBUFT ") + strings.Count(n, " : OBUFDS ")
	t.Logf("mimas_v2 pad_ring: %d buffer instances emitted", count)

	// DEFERRED-SCOPE NOTE: the 32 io_p<n>_<m> PIO pads (io_p6_0..io_p9_7) feed
	// the gpio pi/po arrays in the golden and appear as neither buffers nor in
	// the soc; their buffer resolution is deferred to P5d-c. Count any io_p-
	// derived buffer/direct-wire statements we emit so the divergence is
	// documented (not asserted).
	ioP := 0
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "io_p") && (strings.Contains(line, ": OBUF") ||
			strings.Contains(line, ": IBUF") || strings.Contains(line, ": IOBUF") ||
			strings.Contains(line, ": OBUFT") || strings.Contains(line, "<=")) {
			ioP++
		}
	}
	t.Logf("mimas_v2 pad_ring: %d io_p-derived statements (deferred to P5d-c)", ioP)
}
