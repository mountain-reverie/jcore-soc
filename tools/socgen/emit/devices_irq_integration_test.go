package emit

import (
	"os"
	"strings"
	"testing"

	"github.com/j-core/jcore-soc/tools/socgen/board"
	"github.com/j-core/jcore-soc/tools/socgen/elaborate"
	"github.com/j-core/jcore-soc/tools/socgen/vhdl"
)

func TestDevicesIRQMimasV2(t *testing.T) {
	root := os.Getenv("JCORE_SOC_ROOT")
	if root == "" {
		t.Skip("JCORE_SOC_ROOT not set")
	}
	b, lerr := board.Load(root, "mimas_v2", "")
	if b == nil || b.Design == nil {
		t.Fatalf("board.Load: %v", lerr)
	}
	res, rerr := elaborate.Elaborate(b)
	if rerr != nil {
		t.Logf("elaborate notes: %v", rerr)
	}
	out, derr := Devices(res)
	if derr != nil {
		t.Logf("emit notes: %v", derr)
	}
	if _, perr := vhdl.ParseFile(vhdl.NewFileSet(), "devices.vhd", []byte(out)); perr != nil {
		t.Fatalf("emitted devices.vhd does not re-parse: %v\n%s", perr, out)
	}
	n := strings.Join(strings.Fields(out), " ")
	for _, w := range []string{
		"signal irqs0 : std_logic_vector(7 downto 0) := (others => '0')",
		`vector_numbers => (x"00", x"12", x"00", x"14", x"15", x"00", x"00", x"00")`,
		"irq_i => irqs0",
		"irq => irqs0(4)",
		"int => irqs0(1)",
		"int0 => irqs0(3)",
		"int1 => open",
	} {
		if !strings.Contains(n, strings.Join(strings.Fields(w), " ")) {
			t.Errorf("devices.vhd IRQ missing %q", w)
		}
	}
	t.Logf("mimas_v2 devices.vhd IRQ wiring OK")
}
