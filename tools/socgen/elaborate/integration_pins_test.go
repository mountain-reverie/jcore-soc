package elaborate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/j-core/jcore-soc/tools/socgen/board"
	"github.com/j-core/jcore-soc/tools/socgen/design"
)

// 3-board pin PARSING (design layer only — no make/VHDL needed): every migrated
// board's pins: block + .pins file must parse.
func TestPinsParseAllBoards(t *testing.T) {
	root := os.Getenv("JCORE_SOC_ROOT")
	if root == "" {
		t.Skip("JCORE_SOC_ROOT not set")
	}
	for _, name := range []string{"mimas_v2", "microboard", "turtle_1v0"} {
		yaml := filepath.Join(root, "targets", "boards", name, "design.yaml")
		d, errs := design.Load(yaml)
		if d == nil || d.Pins == nil {
			t.Errorf("%s: no pins parsed (errs: %v)", name, errs)
			continue
		}
		t.Logf("%s: %d rules, %d pins parsed", name, len(d.Pins.Rules), len(d.Pins.Pins))
		if len(d.Pins.Pins) == 0 {
			t.Errorf("%s: zero pins parsed", name)
		}
	}
}

// mimas_v2 net-list JOIN (full elaborate via board.Load + make): pins drive the
// previously-undriven signals.
func TestPinsNetlistMimasV2(t *testing.T) {
	root := os.Getenv("JCORE_SOC_ROOT")
	if root == "" {
		t.Skip("JCORE_SOC_ROOT not set")
	}
	b, _ := board.Load(root, "mimas_v2")
	if b.Design == nil || b.Library == nil {
		t.Skip("board.Load incomplete")
	}
	res, errs := Elaborate(b)
	undriven := 0
	for _, e := range errs {
		if strings.Contains(e.Error(), "nothing drives signal") {
			undriven++
		}
	}
	t.Logf("mimas_v2 with pins: %d devices, %d signals, %d pins, %d undriven (P4d baseline was 64)",
		len(res.Devices), len(res.Signals), len(res.Pins), undriven)
	if len(res.Pins) == 0 {
		t.Fatal("no pins resolved")
	}
	// the join must drive signals: undriven count must drop markedly below the P4d baseline of 64.
	if undriven >= 64 {
		t.Errorf("pins did not reduce undriven count: got %d (baseline 64)", undriven)
	}
	// a pin must drive a signal that a device/top/padring consumes (the net-list now spans pins).
	pinDrivesSignal := false
	for _, s := range res.Signals {
		hasPinOut, hasOtherIn := false, false
		for _, pr := range s.Ports {
			if pr.Context.Kind == "pin" && pr.Dir == "out" {
				hasPinOut = true
			}
			if pr.Context.Kind != "pin" && (pr.Dir == "in" || pr.Dir == "inout") {
				hasOtherIn = true
			}
		}
		if hasPinOut && hasOtherIn {
			pinDrivesSignal = true
			break
		}
	}
	if !pinDrivesSignal {
		t.Error("expected a pin to drive a device/top/padring-consumed signal")
	}
	// differential ddr_clk: two pin drivers (pos/neg) must NOT produce a multi-driver error.
	for _, e := range errs {
		if strings.Contains(e.Error(), `signal "ddr_clk" is driven by multiple ports`) {
			t.Errorf("ddr_clk differential pair wrongly rejected: %v", e)
		}
	}
	// the P4d flash.cs proof remains intact.
	for _, dev := range res.Devices {
		for _, p := range dev.Ports {
			if dev.Name == "flash" && p.Name == "cs" {
				if p.Type.Left == nil || *p.Type.Left != 1 || p.Type.Dir != "downto" {
					t.Errorf("flash.cs no longer (1 downto 0): %s", p.Type.String())
				}
			}
		}
	}
}
