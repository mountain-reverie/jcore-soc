package elaborate

import (
	"testing"

	"github.com/j-core/jcore-soc/tools/socgen/board"
	"github.com/j-core/jcore-soc/tools/socgen/design"
)

func TestElaborateNetlist(t *testing.T) {
	lib := buildLib(t,
		`entity e is port (clk : in std_logic; q : out std_logic); end entity;`,
		`architecture a of e is begin end architecture;`)
	d := &design.Design{
		DeviceClasses: map[string]*design.DeviceClass{"c": {Entity: "e"}},
		Devices: []*design.Device{
			{Class: "c", Name: "d0", Ports: map[string]design.Value{"clk": {Kind: design.KindExpr, Text: "sysclk"}, "q": {Kind: design.KindExpr, Text: "shared"}}},
			{Class: "c", Name: "d1", Ports: map[string]design.Value{"clk": {Kind: design.KindExpr, Text: "sysclk"}}}, // q autogen d1_q
		},
		ZeroSignals: []string{"shared_in_only"},
	}
	res, errs := Elaborate(&board.Board{Name: "b", Design: d, Library: lib})
	if len(errs) != 0 {
		t.Fatalf("errs: %v", errs)
	}
	// sysclk has 2 ports (both d0.clk and d1.clk)
	if s := res.Signals["sysclk"]; s == nil || len(s.Ports) != 2 {
		t.Fatalf("sysclk = %+v", res.Signals["sysclk"])
	}
	// shared driven by d0.q (out)
	if res.Signals["shared"] == nil {
		t.Errorf("shared missing")
	}
}

func TestZeroSignal(t *testing.T) {
	lib := buildLib(t, `entity e is port (en : in std_logic); end entity;`, `architecture a of e is begin end architecture;`)
	d := &design.Design{
		DeviceClasses: map[string]*design.DeviceClass{"c": {Entity: "e"}},
		Devices:       []*design.Device{{Class: "c", Name: "d0", Ports: map[string]design.Value{"en": {Kind: design.KindExpr, Text: "ctrl"}}}},
		ZeroSignals:   []string{"ctrl"}, // undriven (en is :in) -> gets a zero :out driver
	}
	res, _ := Elaborate(&board.Board{Design: d, Library: lib})
	s := res.Signals["ctrl"]
	if s == nil {
		t.Fatal("ctrl missing")
	}
	hasZero := false
	for _, p := range s.Ports {
		if p.Context.Kind == "zero" && p.Dir == "out" {
			hasZero = true
		}
	}
	if !hasZero {
		t.Errorf("ctrl should have a synthetic zero driver: %+v", s.Ports)
	}
}
