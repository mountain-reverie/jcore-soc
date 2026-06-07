package elaborate

import (
	"errors"
	"testing"
)

func dbDev(ports ...*ResolvedPort) *ResolvedDevice {
	return &ResolvedDevice{Name: "d", Class: "c", Ports: ports}
}

func TestClassifyDataBusPorts(t *testing.T) {
	dev := dbDev(
		&ResolvedPort{Name: "db_o", Dir: dirOut, Type: &ResolvedType{Mark: "cpu_data_i_t"}, Kind: KindSignal},
		&ResolvedPort{Name: "db_i", Dir: dirIn, Type: &ResolvedType{Mark: "cpu_data_o_t"}, Kind: KindSignal},
		&ResolvedPort{Name: "clk", Dir: dirIn, Type: &ResolvedType{Mark: "std_logic"}, Kind: KindSignal},
	)
	if err := classifyDataBus([]*ResolvedDevice{dev}); err != nil {
		t.Fatalf("classify: %v", err)
	}
	if !dev.DataBus {
		t.Error("device should be a data-bus participant")
	}
	byName := map[string]*ResolvedPort{}
	for _, p := range dev.Ports {
		byName[p.Name] = p
	}
	if byName["db_o"].Kind != KindDataBus || byName["db_i"].Kind != KindDataBus {
		t.Errorf("db ports not classified: %v / %v", byName["db_o"].Kind, byName["db_i"].Kind)
	}
	if byName["clk"].Kind != KindSignal {
		t.Error("clk should stay KindSignal")
	}
}

func TestClassifyDataBusNonParticipant(t *testing.T) {
	dev := dbDev(&ResolvedPort{Name: "clk", Dir: dirIn, Type: &ResolvedType{Mark: "std_logic"}, Kind: KindSignal})
	if err := classifyDataBus([]*ResolvedDevice{dev}); err != nil {
		t.Fatalf("classify: %v", err)
	}
	if dev.DataBus {
		t.Error("plain device must not be a data-bus participant")
	}
}

func TestClassifyDataBusMalformed(t *testing.T) {
	// has the out (read) port but not the in (write) port -> malformed
	dev := dbDev(&ResolvedPort{Name: "db_o", Dir: dirOut, Type: &ResolvedType{Mark: "cpu_data_i_t"}, Kind: KindSignal})
	err := classifyDataBus([]*ResolvedDevice{dev})
	if err == nil || !errors.Is(err, ErrDataBusPorts) {
		t.Fatalf("want ErrDataBusPorts, got %v", err)
	}
}

func TestClassifyDataBusNilType(t *testing.T) {
	dev := dbDev(&ResolvedPort{Name: "x", Dir: dirOut, Type: nil, Kind: KindSignal})
	if err := classifyDataBus([]*ResolvedDevice{dev}); err != nil {
		t.Fatalf("classify: %v", err)
	}
	if dev.DataBus {
		t.Error("device with a nil-Type port must not be a data-bus participant")
	}
}
