package emit

import (
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/j-core/jcore-soc/tools/socgen/board"
	"github.com/j-core/jcore-soc/tools/socgen/elaborate"
	"github.com/j-core/jcore-soc/tools/socgen/vhdl"
)

func TestPadRingPioMimasV2(t *testing.T) {
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
	n := strings.Join(strings.Fields(out), " ")

	// loopback bits 0..18, constant ties 19..31.
	for i := 0; i <= 18; i++ {
		w := "pi(" + strconv.Itoa(i) + ") <= po(" + strconv.Itoa(i) + ")"
		if !strings.Contains(n, w) {
			t.Errorf("missing loopback %q", w)
		}
	}
	for i := 19; i <= 31; i++ {
		w := "pi(" + strconv.Itoa(i) + ") <= '0'"
		if !strings.Contains(n, w) {
			t.Errorf("missing const tie %q", w)
		}
	}

	// PIO block must be positioned BEFORE the buffer section (direct-wire + first OBUF).
	piIdx := strings.Index(n, "pi(0) <= po(0)")
	clkIdx := strings.Index(n, "clk_100mhz <= pin_clk_100mhz")
	obufIdx := strings.Index(n, "obuf_led0 : OBUF")
	if piIdx < 0 || clkIdx < 0 || obufIdx < 0 {
		t.Fatalf("anchors not found: pi=%d clk=%d obuf=%d", piIdx, clkIdx, obufIdx)
	}
	if !(piIdx < clkIdx && piIdx < obufIdx) {
		t.Errorf("PIO block must precede buffers: pi=%d clk=%d obuf=%d", piIdx, clkIdx, obufIdx)
	}
	t.Logf("mimas_v2 pad_ring PIO: 19 loopback + 13 const ties, before buffers")
}
