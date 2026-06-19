package emit

import (
	"errors"
	"strings"
	"testing"

	"github.com/j-core/jcore-soc/tools/socgen/elaborate"
	"github.com/j-core/jcore-soc/tools/socgen/iface"
	"github.com/j-core/jcore-soc/tools/socgen/vhdl"
)

func TestSocEntityPorts(t *testing.T) {
	res := &elaborate.Resolution{
		Signals: map[string]*elaborate.Signal{
			"clk_sys": {Name: "clk_sys", Type: &elaborate.ResolvedType{Mark: "std_logic"}},
			"po":      {Name: "po", Type: &elaborate.ResolvedType{Mark: "std_logic_vector", Left: iptr(31), Right: iptr(0), Dir: "downto"}},
		},
		SignalLocations: &elaborate.SignalLocations{
			PadringTop: []elaborate.PortLoc{{Name: "clk_sys", Dir: "in"}, {Name: "po", Dir: "out"}},
		},
	}
	ports := socEntityPorts(res)
	got := map[string]string{}
	for _, p := range ports {
		got[p.Names[0]] = p.Mode + " " + p.SubtypeMark
	}
	if got["clk_sys"] != "in std_logic" || got["po"] != "out std_logic_vector" {
		t.Errorf("soc ports = %v", got)
	}
}

func TestTopInstStmtEntityVsConfig(t *testing.T) {
	cfg := &elaborate.ResolvedEntity{
		Name: "cpus", Entity: &iface.Entity{Name: "cpus"},
		Config: &iface.Configuration{Name: "one_cpu_decode_rom_fpga"},
		Ports:  []*elaborate.ResolvedPort{{Name: "clk", Kind: elaborate.KindSignal, GlobalSignal: "clk_sys"}},
	}
	ci := topInstStmt(cfg, nil)
	if ci.UnitKind != vhdl.CONFIGURATION || ci.Unit != "work.one_cpu_decode_rom_fpga" || ci.Arch != "" {
		t.Errorf("config inst = %+v", ci)
	}
	if ci.Label != "cpus" {
		t.Errorf("config label = %q", ci.Label)
	}
	ent := &elaborate.ResolvedEntity{
		Name: "ddr_ctrl", Entity: &iface.Entity{Name: "ddr_fsm"}, ArchName: "logic",
		Ports: []*elaborate.ResolvedPort{{Name: "clk", Kind: elaborate.KindSignal, GlobalSignal: "clk_sys"}},
	}
	ei := topInstStmt(ent, nil)
	if ei.UnitKind != vhdl.ENTITY || ei.Unit != "work.ddr_fsm" || ei.Arch != "logic" || ei.Label != "ddr_ctrl" {
		t.Errorf("entity inst = %+v", ei)
	}
	// port wired to its global signal
	if len(ei.PortMap) != 1 || ei.PortMap[0].Formal != "clk" || exprText(ei.PortMap[0].Actual) != "clk_sys" {
		t.Errorf("entity port map = %+v", ei.PortMap)
	}
}

func TestDevicesInstStmt(t *testing.T) {
	res := &elaborate.Resolution{
		SignalLocations: &elaborate.SignalLocations{
			TopDevices: []elaborate.PortLoc{{Name: "clk_sys", Dir: "in"}, {Name: "po", Dir: "out"}},
		},
	}
	di := devicesInstStmt(res)
	if di.Label != "devices" || di.UnitKind != vhdl.ENTITY || di.Unit != "work.devices" || di.Arch != "impl" {
		t.Fatalf("devices inst = %+v", di)
	}
	pm := map[string]string{}
	for _, a := range di.PortMap {
		pm[a.Formal] = exprText(a.Actual)
	}
	if pm["clk_sys"] != "clk_sys" || pm["po"] != "po" {
		t.Errorf("devices port map = %v (want name=>name)", pm)
	}
}

func TestSoCAssembly(t *testing.T) {
	res := &elaborate.Resolution{
		TopEntities: map[string]*elaborate.ResolvedEntity{
			"cpus": {Name: "cpus", Entity: &iface.Entity{Name: "cpus"},
				Config: &iface.Configuration{Name: "one_cpu"},
				Ports:  []*elaborate.ResolvedPort{{Name: "clk", Kind: elaborate.KindSignal, GlobalSignal: "clk_sys"}}},
		},
		Signals: map[string]*elaborate.Signal{
			"clk_sys": {Name: "clk_sys", Type: &elaborate.ResolvedType{Mark: "std_logic"}},
			"top_int": {Name: "top_int", Type: &elaborate.ResolvedType{Mark: "std_logic"}},
			"texsig":  {Name: "texsig", Type: &elaborate.ResolvedType{Mark: "std_logic"}},
			"zsig":    {Name: "zsig", Type: &elaborate.ResolvedType{Mark: "std_logic"}, Ports: []*elaborate.SignalPortRef{{Context: elaborate.Context{Kind: "zero", ID: "_zero"}, Dir: "out"}}},
		},
		SignalLocations: &elaborate.SignalLocations{
			PadringTop: []elaborate.PortLoc{{Name: "clk_sys", Dir: "in"}},
			Top:        []string{"top_int"},
			TopExtra:   map[string]string{"texsig": "sig_texsig"}, // output port also read -> alias
		},
	}
	out, err := SoC(res)
	if err != nil {
		t.Fatalf("SoC: %v", err)
	}
	if _, perr := vhdl.ParseFile(vhdl.NewFileSet(), "soc.vhd", []byte(out)); perr != nil {
		t.Fatalf("re-parse: %v\n%s", perr, out)
	}
	for _, want := range []string{
		"entity soc is",
		"clk_sys : in std_logic",              // soc entity port (PadringTop)
		"signal top_int",                      // Top-category internal decl
		"signal sig_texsig",                   // TopExtra alias declared
		"texsig <= sig_texsig",                // TopExtra cond-assign (output port also read)
		"signal zsig",                         // zero-out signal declared
		"cpus : configuration work.one_cpu",   // top-entity instance (config)
		"devices : entity work.devices(impl)", // devices instance
		"zsig <= '0'",                         // zero-out (zeroVal of std_logic)
	} {
		if !strings.Contains(out, want) {
			t.Errorf("SoC output missing %q:\n%s", want, out)
		}
	}
}

func TestSoCUnboundTopEntity(t *testing.T) {
	res := &elaborate.Resolution{
		TopEntities: map[string]*elaborate.ResolvedEntity{
			"orphan": {Name: "orphan"}, // Entity==nil && Config==nil -> unbound
		},
		Signals:         map[string]*elaborate.Signal{},
		SignalLocations: &elaborate.SignalLocations{},
	}
	out, err := SoC(res)
	if err == nil || !errors.Is(err, ErrUnboundEntity) {
		t.Fatalf("want ErrUnboundEntity, got %v", err)
	}
	// best-effort: still emits a parseable soc.vhd (orphan skipped).
	if _, perr := vhdl.ParseFile(vhdl.NewFileSet(), "soc.vhd", []byte(out)); perr != nil {
		t.Fatalf("best-effort output should still parse: %v\n%s", perr, out)
	}
}
