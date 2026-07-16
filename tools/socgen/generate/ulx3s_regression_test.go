package generate

import (
	"os"
	"strings"
	"testing"

	"github.com/j-core/jcore-soc/tools/socgen/board"
	"github.com/j-core/jcore-soc/tools/socgen/elaborate"
)

func TestULX3SDefaultVariantReproducesJ2Binding(t *testing.T) {
	root := os.Getenv("JCORE_SOC_ROOT")
	if root == "" {
		t.Skip("JCORE_SOC_ROOT not set")
	}
	b, err := board.Load(root, "ulx3s", "") // default variant (j2-direct)
	if err != nil || b.Design == nil || b.Design.CPU == nil {
		t.Fatalf("load ulx3s: %v", err)
	}
	res, _ := elaborate.Elaborate(b)
	files, ferr := Build(b, res)
	if ferr != nil {
		t.Fatalf("Build: %v", ferr)
	}
	var cfg string
	for _, f := range files {
		if f.Name == "cpus_config.vhd" {
			cfg = f.Content
		}
	}
	// The default variant must bind the J2 direct synth config, matching the
	// retired hand-written one_cpu_m0_direct_fpga.
	if !strings.Contains(cfg, "for one_cpu_m0") ||
		!strings.Contains(cfg, "use configuration work.cpu_synth_direct;") {
		t.Errorf("default variant binding regressed:\n%s", cfg)
	}
	if strings.Contains(cfg, "generic map") {
		t.Errorf("J2 default must not bind PRIV_ARCH:\n%s", cfg)
	}
}

// boardDTS builds the given ulx3s variant and returns the generated board.dts.
func boardDTS(t *testing.T, variant string) string {
	t.Helper()
	root := os.Getenv("JCORE_SOC_ROOT")
	if root == "" {
		t.Skip("JCORE_SOC_ROOT not set")
	}
	b, err := board.Load(root, "ulx3s", variant)
	if err != nil {
		t.Fatalf("load ulx3s %q: %v", variant, err)
	}
	res, _ := elaborate.Elaborate(b)
	files, ferr := Build(b, res)
	if ferr != nil {
		t.Fatalf("Build %q: %v", variant, ferr)
	}
	for _, f := range files {
		if f.Name == "board.dts" {
			return f.Content
		}
	}
	t.Fatalf("no board.dts generated for %q", variant)
	return ""
}

// The default (j2-direct) board.dts must be generated console-ready for Linux:
// cpu jcore,j2, a serial0 alias, chosen stdout-path/bootargs, and the uart
// baud/clock the uartlite driver reads. Guards the SP3-a/SP3-c DT generation so
// a socgen change can't silently drop the console or the cpu compatible.
func TestULX3SBoardDTSConsole(t *testing.T) {
	dts := boardDTS(t, "") // default = j2-direct
	for _, want := range []string{
		`compatible = "jcore,j2"`,
		`serial0 = &uart;`,
		`stdout-path = "serial0:115200n8";`,
		`bootargs = "console=ttyUL0 earlycon";`,
		`current-speed = <115200>;`,
	} {
		if !strings.Contains(dts, want) {
			t.Errorf("board.dts (j2-direct) missing %q:\n%s", want, dts)
		}
	}
}

// The j4-rom variant must generate the J4 cpu compatible from cpu.model, so no
// hand-written/sed'd board-j4.dts is needed.
func TestULX3SJ4RomBoardDTSCpuJ4(t *testing.T) {
	dts := boardDTS(t, "j4-rom")
	if !strings.Contains(dts, `compatible = "jcore,j4"`) {
		t.Errorf("j4-rom board.dts must have cpu compatible jcore,j4:\n%s", dts)
	}
}
