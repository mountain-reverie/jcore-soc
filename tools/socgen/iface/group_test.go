package iface

import (
	"testing"

	"github.com/j-core/jcore-soc/tools/socgen/vhdl"
)

func TestExtractPeripheralBusGroups(t *testing.T) {
	src := `entity cpus is
  port (clk : in std_logic);
  group cpu0 : peripheral_bus(cpu0_periph_dbus_o, cpu0_periph_dbus_i);
  group cpu1 : peripheral_bus(cpu1_periph_dbus_o, cpu1_periph_dbus_i);
end entity;`
	df, perr := vhdl.ParseFile(vhdl.NewFileSet(), "cpus.vhd", []byte(src))
	if perr != nil {
		t.Fatalf("parse: %v", perr)
	}
	lib, err := Extract([]*vhdl.DesignFile{df})
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	e, ok := lib.Entity("cpus")
	if !ok {
		t.Fatal("entity cpus not found")
	}
	if len(e.PeripheralBuses) != 2 {
		t.Fatalf("want 2 peripheral buses, got %d (%+v)", len(e.PeripheralBuses), e.PeripheralBuses)
	}
	got := map[string][]string{}
	for _, pb := range e.PeripheralBuses {
		got[pb.Name] = pb.Ports
	}
	if len(got["cpu0"]) != 2 || got["cpu0"][0] != "cpu0_periph_dbus_o" || got["cpu0"][1] != "cpu0_periph_dbus_i" {
		t.Errorf("cpu0 ports = %v", got["cpu0"])
	}
	if _, ok := got["cpu1"]; !ok {
		t.Errorf("cpu1 bus missing: %v", got)
	}
	if len(got["cpu1"]) != 2 || got["cpu1"][0] != "cpu1_periph_dbus_o" || got["cpu1"][1] != "cpu1_periph_dbus_i" {
		t.Errorf("cpu1 ports = %v", got["cpu1"])
	}
}

func TestExtractNoPeripheralBusGroups(t *testing.T) {
	src := `entity bare is port (clk : in std_logic); end entity;`
	df, perr := vhdl.ParseFile(vhdl.NewFileSet(), "bare.vhd", []byte(src))
	if perr != nil {
		t.Fatalf("parse: %v", perr)
	}
	lib, err := Extract([]*vhdl.DesignFile{df})
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	e, ok := lib.Entity("bare")
	if !ok {
		t.Fatal("entity bare not found")
	}
	if e.PeripheralBuses != nil {
		t.Errorf("expected nil PeripheralBuses, got %v", e.PeripheralBuses)
	}
}
