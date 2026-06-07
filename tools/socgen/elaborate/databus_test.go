package elaborate

import (
	"errors"
	"os"
	"testing"

	"github.com/j-core/jcore-soc/tools/socgen/board"
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

func TestResolvePeripheralBusesSingleMaster(t *testing.T) {
	m, err := resolvePeripheralBuses([]string{"cpu0", "cpu1"}, map[string]bool{"cpu1": false}, "exact")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if m.MasterBus != "cpu0" {
		t.Errorf("master = %q", m.MasterBus)
	}
	if len(m.MuxStages) != 0 {
		t.Errorf("single master must have no mux stages: %+v", m.MuxStages)
	}
	if len(m.Disconnected) != 1 || m.Disconnected[0] != "cpu1" {
		t.Errorf("disconnected = %v", m.Disconnected)
	}
	if m.DecodeMode != "exact" {
		t.Errorf("decode mode = %q", m.DecodeMode)
	}
}

func TestResolvePeripheralBusesDualMaster(t *testing.T) {
	m, err := resolvePeripheralBuses([]string{"cpu0", "cpu1"}, map[string]bool{}, "simple")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if m.MasterBus != "cpu01" {
		t.Errorf("master = %q (want cpu01)", m.MasterBus)
	}
	if len(m.MuxStages) != 1 {
		t.Fatalf("want 1 mux stage, got %d", len(m.MuxStages))
	}
	s := m.MuxStages[0]
	if s.Entity != "multi_master_bus_mux" || s.In1 != "cpu0" || s.In2 != "cpu1" || s.Out != "cpu01" {
		t.Errorf("mux stage = %+v", s)
	}
}

func TestDataBusResolutionMimasV2(t *testing.T) {
	root := os.Getenv("JCORE_SOC_ROOT")
	if root == "" {
		t.Skip("JCORE_SOC_ROOT not set")
	}
	b, lerr := board.Load(root, "mimas_v2")
	if b == nil || b.Design == nil {
		t.Fatalf("board.Load returned unusable board: %v", lerr)
	}
	res, rerr := Elaborate(b)
	if rerr != nil {
		t.Logf("elaborate notes: %v", rerr)
	}
	if res == nil {
		t.Fatal("Elaborate returned nil resolution")
	}
	// data-bus devices classified (aic0, cache_ctrl, flash, gpio, uart0 -> 5)
	n := 0
	for _, dev := range res.Devices {
		if dev.DataBus {
			n++
		}
	}
	if n == 0 {
		t.Fatalf("no data-bus devices classified on mimas_v2 (classify/port-type mismatch?)")
	}
	t.Logf("mimas_v2 data-bus devices: %d", n)
	if res.DataBus == nil {
		t.Fatal("res.DataBus is nil despite data-bus devices")
	}
	if res.DataBus.MasterBus != "cpu0" {
		t.Errorf("MasterBus = %q, want cpu0", res.DataBus.MasterBus)
	}
	if len(res.DataBus.Disconnected) != 1 || res.DataBus.Disconnected[0] != "cpu1" {
		t.Errorf("Disconnected = %v, want [cpu1]", res.DataBus.Disconnected)
	}
	if len(res.DataBus.MuxStages) != 0 {
		t.Errorf("mimas_v2 is single-master; want no mux stages, got %+v", res.DataBus.MuxStages)
	}
}

func TestResolvePeripheralBusesThreeMasters(t *testing.T) {
	m, err := resolvePeripheralBuses([]string{"cpu0", "cpu1", "dmac"}, map[string]bool{}, "simple")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if m.MasterBus != "cpudm" {
		t.Errorf("master = %q (want cpudm)", m.MasterBus)
	}
	if len(m.MuxStages) != 2 {
		t.Fatalf("want 2 mux stages, got %d (%+v)", len(m.MuxStages), m.MuxStages)
	}
	s0, s1 := m.MuxStages[0], m.MuxStages[1]
	if s0.Entity != "multi_master_bus_mux" || s0.In1 != "cpu0" || s0.In2 != "cpu1" || s0.Out != "cpu01" {
		t.Errorf("stage0 = %+v", s0)
	}
	if s1.Entity != "multi_master_bus_muxff" || s1.In1 != "cpu01" || s1.In2 != "dmac" || s1.Out != "cpudm" {
		t.Errorf("stage1 = %+v", s1)
	}
}
