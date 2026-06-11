package emit

import (
	"os"
	"path/filepath"
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
