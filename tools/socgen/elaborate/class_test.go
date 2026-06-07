package elaborate

import (
	"strings"
	"testing"

	"github.com/j-core/jcore-soc/tools/socgen/design"
	"github.com/j-core/jcore-soc/tools/socgen/iface"
	"github.com/j-core/jcore-soc/tools/socgen/vhdl"
)

// buildLib parses VHDL sources into an iface.Library.
func buildLib(t *testing.T, srcs ...string) *iface.Library {
	t.Helper()
	var files []*vhdl.DesignFile
	for i, s := range srcs {
		df, err := vhdl.ParseFile(vhdl.NewFileSet(), "t.vhd", []byte(s))
		if err != nil {
			t.Fatalf("parse src %d: %v", i, err)
		}
		files = append(files, df)
	}
	lib, _ := iface.Extract(files)
	return lib
}

func TestResolveClassSingleArch(t *testing.T) {
	lib := buildLib(t,
		`entity uartlitedb is port (clk : in std_logic); end entity;`,
		`architecture rtl of uartlitedb is begin end architecture;`)
	rc, errs := resolveClass("uartlite", &design.DeviceClass{Entity: "uartlitedb"}, lib, nil)
	if len(errs) != 0 {
		t.Fatalf("errs: %v", errs)
	}
	if rc.Entity == nil || rc.Entity.Name != "uartlitedb" || rc.ArchName != "rtl" {
		t.Errorf("rc = %+v (arch %q)", rc, rc.ArchName)
	}
}

func TestResolveClassErrors(t *testing.T) {
	// two architectures -> ambiguous
	lib2 := buildLib(t,
		`entity e is end entity;`,
		`architecture a1 of e is begin end architecture;`,
		`architecture a2 of e is begin end architecture;`)
	if _, errs := resolveClass("c", &design.DeviceClass{Entity: "e"}, lib2, nil); len(errs) == 0 || !strings.Contains(errs[0].Error(), "single architecture") {
		t.Errorf("want ambiguous-arch error, got %v", errs)
	}
	// zero architectures
	lib0 := buildLib(t, `entity e is end entity;`)
	if _, errs := resolveClass("c", &design.DeviceClass{Entity: "e"}, lib0, nil); len(errs) == 0 || !strings.Contains(errs[0].Error(), "any architecture") {
		t.Errorf("want no-arch error, got %v", errs)
	}
	// unknown entity
	if _, errs := resolveClass("c", &design.DeviceClass{Entity: "ghost"}, lib0, nil); len(errs) == 0 || !strings.Contains(errs[0].Error(), "unable to map") {
		t.Errorf("want entity error, got %v", errs)
	}
}

func TestResolveClassConfiguration(t *testing.T) {
	lib := buildLib(t,
		`entity cpu is end entity;`,
		`architecture rtl of cpu is begin end architecture;`,
		`configuration cpu_cfg of cpu is for rtl end for; end configuration;`)
	rc, errs := resolveClass("c", &design.DeviceClass{Entity: "cpu", Configuration: "cpu_cfg"}, lib, nil)
	if len(errs) != 0 {
		t.Fatalf("errs: %v", errs)
	}
	if rc.Config == nil || rc.Config.Name != "cpu_cfg" || rc.ArchName != "rtl" {
		t.Errorf("config resolution = %+v", rc)
	}
}

func TestResolveClassExplicitArch(t *testing.T) {
	lib := buildLib(t,
		`entity e is end entity;`,
		`architecture a1 of e is begin end architecture;`,
		`architecture a2 of e is begin end architecture;`)
	// two archs, but explicit selection picks a2 (no ambiguity error)
	rc, errs := resolveClass("c", &design.DeviceClass{Entity: "e", Architecture: "a2"}, lib, nil)
	if len(errs) != 0 {
		t.Fatalf("explicit arch should resolve cleanly: %v", errs)
	}
	if rc.ArchName != "a2" {
		t.Errorf("ArchName = %q want a2", rc.ArchName)
	}
}

func TestResolveClassArchConfigMismatch(t *testing.T) {
	lib := buildLib(t,
		`entity e is end entity;`,
		`architecture rtl of e is begin end architecture;`,
		`architecture other of e is begin end architecture;`,
		`configuration ecfg of e is for rtl end for; end configuration;`)
	// configuration selects rtl, but explicit architecture says "other" -> mismatch
	_, errs := resolveClass("c", &design.DeviceClass{Entity: "e", Architecture: "other", Configuration: "ecfg"}, lib, nil)
	found := false
	for _, e := range errs {
		if strings.Contains(e.Error(), "mismatch") {
			found = true
		}
	}
	if !found {
		t.Errorf("want arch/config mismatch error, got %v", errs)
	}
}

func iptr(i int) *int { return &i }

func TestResolveRegs(t *testing.T) {
	lib := buildLib(t, `entity e is end entity;`, `architecture a of e is begin end architecture;`)
	dc := &design.DeviceClass{Entity: "e", Regs: []*design.Reg{
		{Name: "RX"},                                    // addr 0, width 4 -> [0,3]
		{Name: "tx", Width: iptr(4)},                   // addr 4 -> [4,7]
		{Name: "ctrl", Addr: iptr(12), Width: iptr(4)}, // explicit [12,15]
	}}
	rc, errs := resolveClass("uartlite", dc, lib, nil)
	if len(errs) != 0 {
		t.Fatalf("errs: %v", errs)
	}
	if len(rc.Regs) != 3 {
		t.Fatalf("regs: %d", len(rc.Regs))
	}
	if rc.Regs[0].Name != "rx" || rc.Regs[0].ByteRange != [2]int{0, 3} {
		t.Errorf("reg0 = %+v", rc.Regs[0])
	}
	if rc.Regs[1].ByteRange != [2]int{4, 7} {
		t.Errorf("reg1 = %+v", rc.Regs[1])
	}
	if rc.Regs[2].ByteRange != [2]int{12, 15} {
		t.Errorf("reg2 = %+v", rc.Regs[2])
	}
	// maxAddr = 16 -> ceil(log2 16)-1 = 4-1 = 3
	if rc.LeftAddrBit != 3 {
		t.Errorf("left-addr-bit = %d want 3", rc.LeftAddrBit)
	}
}

func TestResolveRegsOverlap(t *testing.T) {
	lib := buildLib(t, `entity e is end entity;`, `architecture a of e is begin end architecture;`)
	dc := &design.DeviceClass{Entity: "e", Regs: []*design.Reg{
		{Name: "a", Addr: iptr(0), Width: iptr(8)}, // [0,7]
		{Name: "b", Addr: iptr(4), Width: iptr(4)}, // [4,7] overlaps a
	}}
	_, errs := resolveClass("c", dc, lib, nil)
	if len(errs) == 0 {
		t.Fatal("want register-overlap error")
	}
}

func TestResolveRegsLeftAddrTooSmall(t *testing.T) {
	lib := buildLib(t, `entity e is end entity;`, `architecture a of e is begin end architecture;`)
	dc := &design.DeviceClass{Entity: "e", LeftAddrBit: 1, Regs: []*design.Reg{
		{Name: "a", Addr: iptr(0), Width: iptr(16)}, // needs left-addr-bit >= ceil(log2 16)-1 = 3
	}}
	_, errs := resolveClass("c", dc, lib, nil)
	if len(errs) == 0 {
		t.Fatal("want left-addr-bit-too-small error")
	}
}
