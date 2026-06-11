package emit

import (
	"os"
	"strings"
	"testing"

	"github.com/j-core/jcore-soc/tools/socgen/board"
	"github.com/j-core/jcore-soc/tools/socgen/elaborate"
)

func TestStructuralFormatMimasV2(t *testing.T) {
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
	dev, derr := Devices(res)
	soc, serr := SoC(res)
	pad, perr := PadRing(res)
	if derr != nil || serr != nil || perr != nil {
		t.Logf("emit notes: devices=%v soc=%v padring=%v", derr, serr, perr)
	}

	for name, out := range map[string]string{"devices": dev, "soc": soc, "pad_ring": pad} {
		// Banner is the exact first 8 lines.
		if !strings.HasPrefix(out, fileBanner) {
			t.Errorf("%s.vhd does not start with the header banner:\n%.200s", name, out)
		}
		// Bare end; for entity/architecture; no end entity;/end architecture;/end function;.
		for _, bad := range []string{"end entity;", "end architecture;", "end function;"} {
			if strings.Contains(out, bad) {
				t.Errorf("%s.vhd still contains %q", name, bad)
			}
		}
		if !strings.Contains(out, "\nend;\n") {
			t.Errorf("%s.vhd missing a bare `end;`", name)
		}
	}
	// 4-space indentation, golden-exact lines (soc.vhd entity port block).
	for _, line := range []string{"\n    port (\n", "\n        clk10 : in std_logic;\n"} {
		if !strings.Contains(soc, line) {
			t.Errorf("soc.vhd missing 4-space-indented line %q", line)
		}
	}
	// devices.vhd's decode_address function closes with an indented bare `end;`
	// (positively covers the function end; case, not just entity/architecture).
	if !strings.Contains(dev, "\n    end;\n") {
		t.Errorf("devices.vhd missing the function's indented bare `end;`")
	}
}
