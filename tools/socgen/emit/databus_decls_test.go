package emit

import (
	"strings"
	"testing"

	"github.com/j-core/jcore-soc/tools/socgen/elaborate"
	"github.com/j-core/jcore-soc/tools/socgen/iface"
	"github.com/j-core/jcore-soc/tools/socgen/vhdl"
)

func u64(v uint64) *uint64 { return &v }

func TestDevicesDataBusDecls(t *testing.T) {
	dbPorts := func() []*elaborate.ResolvedPort {
		return []*elaborate.ResolvedPort{
			{Name: "db_o", Dir: "out", Type: &elaborate.ResolvedType{Mark: "cpu_data_i_t"}, Kind: elaborate.KindDataBus},
			{Name: "db_i", Dir: "in", Type: &elaborate.ResolvedType{Mark: "cpu_data_o_t"}, Kind: elaborate.KindDataBus},
		}
	}
	res := &elaborate.Resolution{
		Classes: map[string]*elaborate.ResolvedClass{
			"gpio": {Entity: &iface.Entity{Name: "gpio2"}, ArchName: "rtl", LeftAddrBit: 3},
			"uart": {Entity: &iface.Entity{Name: "uartlitedb"}, ArchName: "rtl", LeftAddrBit: 3},
		},
		Devices: []*elaborate.ResolvedDevice{
			{Name: "gpio", Class: "gpio", DataBus: true, BaseAddr: u64(0xA0000000), Ports: dbPorts()},
			{Name: "uart0", Class: "uart", DataBus: true, BaseAddr: u64(0xA0000100), Ports: dbPorts()},
		},
		Signals: map[string]*elaborate.Signal{},
		DataBus: &elaborate.PeripheralBusModel{MasterBus: "cpu0", Disconnected: []string{"cpu1"}, DecodeMode: "exact"},
	}
	out, err := Devices(res)
	if err != nil {
		t.Logf("emit notes: %v", err)
	}
	// re-parse: valid VHDL.
	if _, perr := vhdl.ParseFile(vhdl.NewFileSet(), "devices.vhd", []byte(out)); perr != nil {
		t.Fatalf("output does not re-parse: %v\n%s", perr, out)
	}
	for _, want := range []string{
		"type device_t is (NONE, DEV_GPIO, DEV_UART0)",
		"signal active_dev : device_t",
		"data_bus_i_t",
		"signal devs_bus_i",
		"function decode_address",
		"active_dev <= decode_address(cpu0_periph_dbus_o.a)",
		"cpu0_periph_dbus_i <= devs_bus_i(active_dev)",
		"bus_split",
		"devs_bus_i(NONE) <= loopback_bus(devs_bus_o(NONE))",
		"cpu1_periph_dbus_i <= loopback_bus(cpu1_periph_dbus_o)",
		"use work.data_bus_pack.all",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("emitted devices.vhd missing %q\n---\n%s", want, out)
		}
	}
	// data-bus device port wiring replaced `open`.
	if !strings.Contains(out, "db_o => devs_bus_i(DEV_GPIO)") || !strings.Contains(out, "db_i => devs_bus_o(DEV_GPIO)") {
		t.Errorf("gpio db ports not wired to devs_bus arrays:\n%s", out)
	}
}
