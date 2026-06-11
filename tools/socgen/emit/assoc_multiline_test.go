package emit

import (
	"os"
	"strings"
	"testing"

	"github.com/j-core/jcore-soc/tools/socgen/board"
	"github.com/j-core/jcore-soc/tools/socgen/elaborate"
)

func TestAssocMultilineMimasV2(t *testing.T) {
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
	pad, _ := PadRing(res)

	gpio := "    gpio : entity work.pio(beh)\n" +
		"        port map (\n" +
		"            clk_bus => clk_sys,\n" +
		"            db_i => devs_bus_o(DEV_GPIO),\n" +
		"            db_o => devs_bus_i(DEV_GPIO),\n" +
		"            irq => irqs0(4),\n" +
		"            p_i => pi,\n" +
		"            p_o => po,\n" +
		"            reset => reset\n" +
		"        );"
	if !strings.Contains(dev, gpio) {
		t.Errorf("devices.vhd gpio multi-line block mismatch; got around:\n%s", around(dev, "gpio : entity"))
	}

	obuf := "    obuf_led0 : OBUF\n" +
		"        generic map (\n" +
		"            DRIVE => 24,\n" +
		"            IOSTANDARD => \"LVCMOS33\"\n" +
		"        )\n" +
		"        port map (\n" +
		"            I => po(0),\n" +
		"            O => pin_led0\n" +
		"        );"
	if !strings.Contains(pad, obuf) {
		t.Errorf("pad_ring.vhd obuf_led0 multi-line block mismatch; got around:\n%s", around(pad, "obuf_led0 : OBUF"))
	}
}

// around returns a window of text around the first occurrence of marker.
func around(s, marker string) string {
	i := strings.Index(s, marker)
	if i < 0 {
		return "(marker not found)"
	}
	end := i + 400
	if end > len(s) {
		end = len(s)
	}
	return s[i:end]
}
