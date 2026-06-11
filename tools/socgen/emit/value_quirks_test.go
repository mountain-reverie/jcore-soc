package emit

import (
	"os"
	"strings"
	"testing"

	"github.com/j-core/jcore-soc/tools/socgen/board"
	"github.com/j-core/jcore-soc/tools/socgen/elaborate"
)

func TestValueQuirksMimasV2(t *testing.T) {
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
	dev, _ := Devices(res)
	soc, _ := SoC(res)
	pad, _ := PadRing(res)

	// Fix A: bool literals uppercase.
	for _, w := range []string{"insert_inst_delay_boot_mem => FALSE", "resync_en => TRUE"} {
		if !strings.Contains(soc, w) {
			t.Errorf("soc.vhd missing %q", w)
		}
	}
	if strings.Contains(soc, "=> false") || strings.Contains(soc, "=> true") {
		t.Errorf("soc.vhd still has a lowercase bool literal")
	}
	if !strings.Contains(dev, "rtc_sec_length34b => FALSE") {
		t.Errorf("devices.vhd aic generic should be FALSE")
	}

	// Fix B: space before the subprogram param paren.
	if !strings.Contains(dev, "function decode_address (addr ") {
		t.Errorf("devices.vhd decode_address should have a space before (")
	}

	// Fix C: lowercased loc value.
	if !strings.Contains(pad, `signal is "v10"`) {
		t.Errorf("pad_ring.vhd loc value should be lowercased to v10")
	}
	if strings.Contains(pad, `signal is "V10"`) {
		t.Errorf("pad_ring.vhd loc value must not be uppercase V10")
	}
}
