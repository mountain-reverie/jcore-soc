package emit

import (
	"strings"
	"testing"

	"github.com/j-core/jcore-soc/tools/socgen/elaborate"
	"github.com/j-core/jcore-soc/tools/socgen/vhdl"
)

func bd(name string, base uint64, leftBit int) busDevice {
	return busDevice{Name: name, BaseAddr: base, LeftAddrBit: leftBit}
}

func TestDevicePrefixExact(t *testing.T) {
	// base 0xABCD0000, leftBit 3 -> drop low 4 bits -> 28-bit prefix.
	p := devicePrefix(bd("gpio", 0xABCD0000, 3), false)
	if len(p) != 28 {
		t.Fatalf("exact prefix len = %d, want 28", len(p))
	}
	// 0xABCD = 1010 1011 1100 1101 -> the prefix starts with those 16 bits.
	if !strings.HasPrefix(p, "1010101111001101") {
		t.Errorf("prefix = %q", p)
	}
}

func TestDecodeAddressStructure(t *testing.T) {
	devs := []busDevice{
		bd("gpio", 0xA0000000, 3),
		bd("uart0", 0xA0000100, 3),
	}
	lits := map[string]string{"gpio": "DEV_GPIO", "uart0": "DEV_UART0", "none": "NONE"}
	fn := decodeFunction(devs, lits, false) // *vhdl.SubprogramBody
	if fn.Designator != "decode_address" || fn.ReturnMark != "device_t" {
		t.Fatalf("spec = %q -> %q", fn.Designator, fn.ReturnMark)
	}
	// render inside a throwaway architecture and re-parse to prove valid VHDL.
	df := &vhdl.DesignFile{Units: []vhdl.DesignUnit{
		&vhdl.ArchitectureBody{Name: "impl", Entity: "e", Decls: []vhdl.Decl{fn}},
	}}
	out := vhdl.Print(df)
	for _, want := range []string{"function decode_address", "return DEV_GPIO", "return DEV_UART0", "return NONE"} {
		if !strings.Contains(out, want) {
			t.Errorf("decode fn missing %q:\n%s", want, out)
		}
	}
	if _, perr := vhdl.ParseFile(vhdl.NewFileSet(), "t.vhd", []byte(out)); perr != nil {
		t.Fatalf("decode fn does not re-parse: %v\n%s", perr, out)
	}
}

func TestDecodeAddressZeroDevices(t *testing.T) {
	fn := decodeFunction(nil, map[string]string{"none": "NONE"}, false)
	df := &vhdl.DesignFile{Units: []vhdl.DesignUnit{
		&vhdl.ArchitectureBody{Name: "impl", Entity: "e", Decls: []vhdl.Decl{fn}},
	}}
	out := vhdl.Print(df)
	if !strings.Contains(out, "return NONE") {
		t.Errorf("zero-device decode must return NONE:\n%s", out)
	}
	if strings.Contains(out, "if ") {
		t.Errorf("zero-device decode must have no if statement:\n%s", out)
	}
}

func TestDecodeAddressExactStructure(t *testing.T) {
	devs := []busDevice{
		bd("gpio", 0xABCD0000, 3),
		bd("flash", 0xABCD0040, 2),
		bd("aic0", 0xABCD0200, 5),
	}
	lits := map[string]string{"gpio": "DEV_GPIO", "flash": "DEV_FLASH", "aic0": "DEV_AIC0", "none": "NONE"}
	fn := decodeFunction(devs, lits, false)
	df := &vhdl.DesignFile{Units: []vhdl.DesignUnit{
		&vhdl.ArchitectureBody{Name: "impl", Entity: "e", Decls: []vhdl.Decl{fn}},
	}}
	out := vhdl.Print(df)
	for _, want := range []string{
		"addr(27 downto 10) = \"101111001101000000\"", // top-level >4-bit prefix wrap
		"addr(6 downto 3) = \"1000\"",                 // flash multi-bit slice (lo = hi-len+1)
		"elsif",                                        // if/elsif path exercised
		"return DEV_FLASH",
		"return DEV_AIC0",
		"return NONE",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("decode output missing %q:\n%s", want, out)
		}
	}
	// re-parse for validity.
	if _, perr := vhdl.ParseFile(vhdl.NewFileSet(), "t.vhd", []byte(out)); perr != nil {
		t.Fatalf("decode does not re-parse: %v\n%s", perr, out)
	}
}

func TestDecodeAddressSimpleModeTrims(t *testing.T) {
	devs := []busDevice{
		bd("a", 0xA0000000, 3),
		bd("b", 0xA0000080, 3),
	}
	lits := map[string]string{"a": "DEV_A", "b": "DEV_B", "none": "NONE"}
	exact := renderFn(t, decodeFunction(devs, lits, false))
	simple := renderFn(t, decodeFunction(devs, lits, true))
	// both must decode both devices...
	for _, out := range []string{exact, simple} {
		for _, w := range []string{"return DEV_A", "return DEV_B", "return NONE"} {
			if !strings.Contains(out, w) {
				t.Errorf("missing %q:\n%s", w, out)
			}
		}
	}
	// ...and simple mode must compare FEWER total address-bit positions than exact
	// (trimmed suffix). Use the count of "downto" + "addr(" occurrences as a proxy,
	// or assert simple's output is shorter. Simple <= exact in compared-bit span.
	if len(simple) > len(exact) {
		t.Errorf("simple-mode decode should not be longer than exact:\nsimple=%s\nexact=%s", simple, exact)
	}
}

func renderFn(t *testing.T, fn *vhdl.SubprogramBody) string {
	t.Helper()
	df := &vhdl.DesignFile{Units: []vhdl.DesignUnit{
		&vhdl.ArchitectureBody{Name: "impl", Entity: "e", Decls: []vhdl.Decl{fn}},
	}}
	return vhdl.Print(df)
}

func TestMuxChainDualMaster(t *testing.T) {
	res := &elaborate.Resolution{
		DataBus: &elaborate.PeripheralBusModel{
			MasterBus: "cpu01", DecodeMode: "simple",
			MuxStages: []*elaborate.MuxStage{
				{Label: "cpus_mux", Entity: "multi_master_bus_mux", In1: "cpu0", In2: "cpu1", Out: "cpu01"},
			},
		},
	}
	stmts := muxChainStmts(res)
	if len(stmts) != 1 {
		t.Fatalf("want 1 mux instantiation, got %d", len(stmts))
	}
	inst, ok := stmts[0].(*vhdl.InstantiationStmt)
	if !ok || inst.Label != "cpus_mux" || inst.Unit != "work.multi_master_bus_mux" {
		t.Fatalf("mux inst = %+v", stmts[0])
	}
	if inst.UnitKind != vhdl.ENTITY || inst.Arch != "" {
		t.Errorf("want ENTITY with no arch qualifier, got kind=%v arch=%q", inst.UnitKind, inst.Arch)
	}
	pm := map[string]string{}
	for _, a := range inst.PortMap {
		pm[a.Formal] = exprText(a.Actual)
	}
	for f, want := range map[string]string{
		"m1_i": "cpu0_periph_dbus_i", "m1_o": "cpu0_periph_dbus_o",
		"m2_i": "cpu1_periph_dbus_i", "m2_o": "cpu1_periph_dbus_o",
		"slave_i": "cpu01_periph_dbus_i", "slave_o": "cpu01_periph_dbus_o",
		"clk": "clk_sys", "rst": "reset",
	} {
		if pm[f] != want {
			t.Errorf("port %s = %q, want %q", f, pm[f], want)
		}
	}
}

func TestMuxChainThreeMasters(t *testing.T) {
	res := &elaborate.Resolution{
		DataBus: &elaborate.PeripheralBusModel{
			MasterBus: "cpudm", DecodeMode: "simple",
			MuxStages: []*elaborate.MuxStage{
				{Label: "cpus_mux", Entity: "multi_master_bus_mux", In1: "cpu0", In2: "cpu1", Out: "cpu01"},
				{Label: "dmac_mux", Entity: "multi_master_bus_muxff", In1: "cpu01", In2: "dmac", Out: "cpudm"},
			},
		},
	}
	stmts := muxChainStmts(res)
	if len(stmts) != 2 {
		t.Fatalf("want 2 mux instantiations, got %d", len(stmts))
	}
	want := []struct{ label, unit, slaveI string }{
		{"cpus_mux", "work.multi_master_bus_mux", "cpu01_periph_dbus_i"},
		{"dmac_mux", "work.multi_master_bus_muxff", "cpudm_periph_dbus_i"},
	}
	for i, w := range want {
		inst, ok := stmts[i].(*vhdl.InstantiationStmt)
		if !ok || inst.Label != w.label || inst.Unit != w.unit {
			t.Fatalf("stage %d = %+v", i, stmts[i])
		}
		pm := map[string]string{}
		for _, a := range inst.PortMap {
			pm[a.Formal] = exprText(a.Actual)
		}
		if pm["slave_i"] != w.slaveI {
			t.Errorf("stage %d slave_i = %q, want %q", i, pm["slave_i"], w.slaveI)
		}
	}
	// chaining invariant: stage 1's m1 input is stage 0's output bus.
	s1 := stmts[1].(*vhdl.InstantiationStmt)
	pm1 := map[string]string{}
	for _, a := range s1.PortMap {
		pm1[a.Formal] = exprText(a.Actual)
	}
	if pm1["m1_i"] != "cpu01_periph_dbus_i" {
		t.Errorf("stage1 m1_i = %q, want cpu01_periph_dbus_i (chaining)", pm1["m1_i"])
	}
}

func TestMuxChainSingleMasterEmpty(t *testing.T) {
	res := &elaborate.Resolution{DataBus: &elaborate.PeripheralBusModel{MasterBus: "cpu0"}}
	if s := muxChainStmts(res); len(s) != 0 {
		t.Errorf("single master must emit no mux instantiations, got %d", len(s))
	}
}
