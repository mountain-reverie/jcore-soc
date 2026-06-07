package elaborate

import (
	"strings"
	"testing"

	"github.com/j-core/jcore-soc/tools/socgen/board"
	"github.com/j-core/jcore-soc/tools/socgen/design"
)

func TestDeviceNaming(t *testing.T) {
	lib := buildLib(t, `entity e is end entity;`, `architecture a of e is begin end architecture;`)
	d := &design.Design{
		DeviceClasses: map[string]*design.DeviceClass{"gpio": {Entity: "e"}},
		Devices: []*design.Device{
			{Class: "gpio", Name: "led"}, // explicit
			{Class: "gpio"},              // -> gpio0 (gpio appears twice -> not sole -> gpio0)
			{Class: "gpio"},              // -> gpio1
		},
	}
	res, errs := Devices(&board.Board{Name: "b", Design: d, Library: lib})
	if len(errs) != 0 {
		t.Fatalf("errs: %v", errs)
	}
	names := []string{res.Devices[0].Name, res.Devices[1].Name, res.Devices[2].Name}
	if names[0] != "led" || names[1] != "gpio0" || names[2] != "gpio1" {
		t.Errorf("names = %v", names)
	}
}

func TestDeviceSoleInstanceUsesClassName(t *testing.T) {
	lib := buildLib(t, `entity e is end entity;`, `architecture a of e is begin end architecture;`)
	d := &design.Design{
		DeviceClasses: map[string]*design.DeviceClass{"spi": {Entity: "e"}},
		Devices:       []*design.Device{{Class: "spi"}},
	}
	res, _ := Devices(&board.Board{Design: d, Library: lib})
	if res.Devices[0].Name != "spi" {
		t.Errorf("sole instance name = %q want spi", res.Devices[0].Name)
	}
}

func TestDeviceGenericMergeAndUnknown(t *testing.T) {
	lib := buildLib(t,
		`entity e is generic (width : integer; fast : boolean); port (clk : in std_logic); end entity;`,
		`architecture a of e is begin end architecture;`)
	d := &design.Design{
		DeviceClasses: map[string]*design.DeviceClass{
			"c": {Entity: "e", Generics: map[string]design.Value{"width": {Kind: design.KindInt, Int: 8}}},
		},
		Devices: []*design.Device{
			{Class: "c", Name: "d0", Generics: map[string]design.Value{"width": {Kind: design.KindInt, Int: 16}, "fast": {Kind: design.KindBool, Bool: true}}},
			{Class: "c", Name: "d1", Generics: map[string]design.Value{"bogus": {}}},
		},
	}
	res, errs := Devices(&board.Board{Design: d, Library: lib})
	// d0: instance width=16 overrides class width=8; fast added
	g := res.Devices[0].Generics
	if g["width"].Int != 16 || g["fast"].Bool != true {
		t.Errorf("d0 generics = %+v", g)
	}
	// d1: bogus is unknown
	found := false
	for _, e := range errs {
		if strings.Contains(e.Error(), `unknown generic "bogus"`) {
			found = true
		}
	}
	if !found {
		t.Errorf("want unknown-generic error, got %v", errs)
	}
}

func TestDeviceDuplicateName(t *testing.T) {
	lib := buildLib(t, `entity e is end entity;`, `architecture a of e is begin end architecture;`)
	d := &design.Design{
		DeviceClasses: map[string]*design.DeviceClass{"c": {Entity: "e"}},
		Devices:       []*design.Device{{Class: "c", Name: "x"}, {Class: "c", Name: "x"}},
	}
	_, errs := Devices(&board.Board{Design: d, Library: lib})
	if len(errs) == 0 {
		t.Fatal("want duplicate-name error")
	}
}
