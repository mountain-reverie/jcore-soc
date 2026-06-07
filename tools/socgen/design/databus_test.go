package design

import (
	"testing"

	"gopkg.in/yaml.v3"
)

func TestLoadPeripheralBusesAndDecode(t *testing.T) {
	src := `target: x
peripheral-buses:
  cpu1: false
system:
  data-bus-decode: exact
  dram: [0x10000000, 0x4000000]
`
	var d Design
	if err := yaml.Unmarshal([]byte(src), &d); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if v, ok := d.PeripheralBuses["cpu1"]; !ok || v != false {
		t.Errorf("peripheral-buses cpu1 = %v,%v (want false,true)", v, ok)
	}
	if d.System == nil || d.System.DataBusDecode != "exact" {
		t.Errorf("data-bus-decode = %+v", d.System)
	}
}
