package emit

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/j-core/jcore-soc/tools/socgen/board"
	"github.com/j-core/jcore-soc/tools/socgen/elaborate"
)

// loadMimas loads + elaborates mimas_v2 for the gated comment tests, or skips.
func loadMimas(t *testing.T) *elaborate.Resolution {
	t.Helper()
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
	return res
}

// assertFileEqual compares got to the golden file byte-for-byte, reporting the
// first differing line with a little context.
func assertFileEqual(t *testing.T, got, goldenPath string) {
	t.Helper()
	wantB, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden %s: %v", goldenPath, err)
	}
	want := string(wantB)
	if got == want {
		return
	}
	gl, wl := strings.Split(got, "\n"), strings.Split(want, "\n")
	for i := 0; i < len(gl) || i < len(wl); i++ {
		var g, w string
		if i < len(gl) {
			g = gl[i]
		}
		if i < len(wl) {
			w = wl[i]
		}
		if g != w {
			t.Errorf("%s differs at line %d:\n got:  %q\n want: %q", goldenPath, i+1, g, w)
			return
		}
	}
	t.Errorf("%s differs (length %d vs %d)", goldenPath, len(got), len(want))
}

func TestSocVhdComplete(t *testing.T) {
	res := loadMimas(t)
	soc, _ := SoC(res)
	assertFileEqual(t, soc, filepath.Join(os.Getenv("JCORE_SOC_ROOT"), "targets/boards/mimas_v2/soc.vhd"))
}

func TestDevicesVhdComplete(t *testing.T) {
	res := loadMimas(t)
	dev, _ := Devices(res)
	assertFileEqual(t, dev, filepath.Join(os.Getenv("JCORE_SOC_ROOT"), "targets/boards/mimas_v2/devices.vhd"))
}

func TestPadRingComments(t *testing.T) {
	res := loadMimas(t)
	pad, _ := PadRing(res)
	lines := strings.Split(pad, "\n")

	// `-- Pin attributes` immediately precedes the first attribute spec.
	pinAttrIdx := -1
	for i, ln := range lines {
		if ln == "    -- Pin attributes" {
			pinAttrIdx = i
			break
		}
	}
	if pinAttrIdx < 0 || pinAttrIdx+1 >= len(lines) ||
		!strings.HasPrefix(lines[pinAttrIdx+1], "    attribute loc of ") {
		t.Errorf("`-- Pin attributes` not directly before the first attribute spec")
	}

	// Each loopback `pi(i) <= po(i)` is led by its group comment, in order.
	want := make([]struct{ comment, assign string }, 0, 19)
	for i := 0; i <= 7; i++ {
		want = append(want, struct{ comment, assign string }{"    -- led", pi(i)})
	}
	for i := 8; i <= 15; i++ {
		want = append(want, struct{ comment, assign string }{"    -- sevensegment", pi(i)})
	}
	for i := 16; i <= 18; i++ {
		want = append(want, struct{ comment, assign string }{"    -- sevensegmentenable", pi(i)})
	}
	for _, w := range want {
		block := w.comment + "\n" + w.assign
		if !strings.Contains(pad, block) {
			t.Errorf("missing comment-led assignment:\n%s", block)
		}
	}
	// The constant range [19 31] (`pi(19) <= '0';`) has no leading comment.
	constLine := "    pi(19) <= '0';"
	if !strings.Contains(pad, constLine) {
		t.Errorf("expected const tie %q in pad_ring output", constLine)
	}
	for i, ln := range lines {
		if ln == constLine && i > 0 && strings.HasPrefix(strings.TrimSpace(lines[i-1]), "--") {
			t.Errorf("pi(19) (const) should have no leading comment")
		}
	}
}

// pi renders the loopback assignment line for bit i as the printer emits it.
func pi(i int) string {
	return "    pi(" + strconv.Itoa(i) + ") <= po(" + strconv.Itoa(i) + ");"
}
