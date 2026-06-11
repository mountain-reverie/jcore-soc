package emit

import (
	"os"
	"strings"
	"testing"

	"github.com/j-core/jcore-soc/tools/socgen/board"
	"github.com/j-core/jcore-soc/tools/socgen/elaborate"
)

func TestAttrAlignMimasV2(t *testing.T) {
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
	pad, _ := PadRing(res)

	// Two byte-exact golden lines: a padded one and the max-width one.
	for _, w := range []string{
		`    attribute loc of pin_clk_100mhz          : signal is "v10";`,
		`    attribute loc of pin_sevensegmentenable2 : signal is "b3";`,
	} {
		if !strings.Contains(pad, w) {
			t.Errorf("pad_ring.vhd missing byte-exact aligned line:\n%q", w)
		}
	}

	// The full attribute-spec block matches golden in order, byte-for-byte.
	golden, gerr := os.ReadFile(root + "/targets/boards/mimas_v2/pad_ring.vhd")
	if gerr != nil {
		t.Fatalf("read golden: %v", gerr)
	}
	if g, o := attrLines(string(golden)), attrLines(pad); g != o {
		t.Errorf("pad_ring.vhd attribute-spec block != golden:\n got:\n%s\nwant:\n%s", o, g)
	}
}

// attrLines returns the `    attribute … of …` spec lines (not the bare decls),
// in order, joined.
func attrLines(s string) string {
	var b strings.Builder
	for _, ln := range strings.Split(s, "\n") {
		if strings.HasPrefix(ln, "    attribute ") && strings.Contains(ln, " of ") {
			b.WriteString(ln)
			b.WriteByte('\n')
		}
	}
	return b.String()
}
