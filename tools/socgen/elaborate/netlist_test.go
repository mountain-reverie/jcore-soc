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
	res, _ := Elaborate(&board.Board{Name: "b", Design: d, Library: lib})
	// sysclk has 2 ports (both d0.clk and d1.clk)
	if s := res.Signals["sysclk"]; s == nil || len(s.Ports) != 2 {
		t.Fatalf("sysclk = %+v", res.Signals["sysclk"])
	}
	// shared driven by d0.q (out)
	if res.Signals["shared"] == nil {
		t.Errorf("shared missing")
	}
}

func TestElaborateAutoNamedDevicePorts(t *testing.T) {
	lib := buildLib(t,
		`entity e is port (clk : in std_logic); end entity;`,
		`architecture a of e is begin end architecture;`)
	d := &design.Design{
		DeviceClasses: map[string]*design.DeviceClass{"gpio": {Entity: "e"}},
		// No Name -> resolveDevices will auto-generate "gpio" (single instance of class)
		Devices: []*design.Device{{Class: "gpio", Ports: map[string]design.Value{"clk": {Kind: design.KindExpr, Text: "myclk"}}}},
	}
	res, _ := Elaborate(&board.Board{Design: d, Library: lib})
	// the instance Ports overlay MUST be applied despite the auto-generated name
	if res.Signals["myclk"] == nil {
		t.Errorf("instance port override dropped for auto-named device; signals: %v", keysOf(res.Signals))
	}
}

func keysOf(m map[string]*Signal) []string {
	ks := []string{}
	for k := range m {
		ks = append(ks, k)
	}
	return ks
}

func TestZeroSignalAlreadyDriven(t *testing.T) {
	lib := buildLib(t,
		`entity e is port (q : out std_logic); end entity;`,
		`architecture a of e is begin end architecture;`)
	d := &design.Design{
		DeviceClasses: map[string]*design.DeviceClass{"c": {Entity: "e"}},
		Devices:       []*design.Device{{Class: "c", Name: "d0", Ports: map[string]design.Value{"q": {Kind: design.KindExpr, Text: "drv"}}}},
		ZeroSignals:   []string{"drv"}, // already driven by d0.q (:out) -> must NOT add a zero driver
	}
	res, _ := Elaborate(&board.Board{Design: d, Library: lib})
	s := res.Signals["drv"]
	if s == nil {
		t.Fatal("drv signal missing")
	}
	for _, p := range s.Ports {
		if p.Context.Kind == "zero" {
			t.Errorf("already-driven signal should not get a synthetic zero driver: %+v", s.Ports)
		}
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

func TestElaborateJoinsTopEntity(t *testing.T) {
	lib := buildLib(t,
		`entity dev is port (clk : in std_logic); end entity;`,
		`architecture a of dev is begin end architecture;`,
		`entity clkgen is port (clk_o : out std_logic); end entity;`,
		`architecture a of clkgen is begin end architecture;`)
	d := &design.Design{
		DeviceClasses: map[string]*design.DeviceClass{"c": {Entity: "dev"}},
		// device 'd0' consumes signal 'sys' on its :in clk
		Devices: []*design.Device{{Class: "c", Name: "d0", Ports: map[string]design.Value{"clk": {Kind: design.KindExpr, Text: "sys"}}}},
		// padring entity drives signal 'sys' on its :out clk_o
		PadringEntities: map[string]*design.TopEntity{
			"gen": {Entity: "clkgen", Ports: map[string]design.Value{"clk_o": {Kind: design.KindExpr, Text: "sys"}}},
		},
	}
	res, errs := Elaborate(&board.Board{Design: d, Library: lib})
	s := res.Signals["sys"]
	if s == nil || len(s.Ports) != 2 {
		t.Fatalf("sys should span device + padring (2 ports): %+v", res.Signals["sys"])
	}
	// the join drove 'sys' -> no "nothing drives sys" error
	for _, e := range errs {
		if e.Error() == `nothing drives signal "sys" used by d0.clk` {
			t.Errorf("sys should be driven by the padring out port; got error: %v", e)
		}
	}
	// the padring entity is recorded on the resolution
	if res.PadringEntities["gen"] == nil || res.PadringEntities["gen"].Entity == nil {
		t.Errorf("padring entity 'gen' not resolved: %+v", res.PadringEntities)
	}
}

func TestElaborateUndrivenWithoutTopDriver(t *testing.T) {
	lib := buildLib(t,
		`entity dev is port (clk : in std_logic); end entity;`,
		`architecture a of dev is begin end architecture;`)
	d := &design.Design{
		DeviceClasses: map[string]*design.DeviceClass{"c": {Entity: "dev"}},
		Devices:       []*design.Device{{Class: "c", Name: "d0", Ports: map[string]design.Value{"clk": {Kind: design.KindExpr, Text: "lonely"}}}},
	}
	_, errs := Elaborate(&board.Board{Design: d, Library: lib})
	found := false
	for _, e := range errs {
		if e.Error() == `nothing drives signal "lonely" used by d0.clk` {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 'nothing drives' for an :in-only signal with no top/padring driver; errs: %v", errs)
	}
}
