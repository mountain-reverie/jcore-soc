package elaborate

import (
	"errors"
	"testing"

	"github.com/j-core/jcore-soc/tools/socgen/design"
	"github.com/j-core/jcore-soc/tools/socgen/iface"
	"github.com/j-core/jcore-soc/tools/socgen/vhdl"
)

// buildLib parses VHDL sources into an iface.Library.
func buildLib(t *testing.T, srcs ...string) *iface.Library {
	t.Helper()
	files := make([]*vhdl.DesignFile, 0, len(srcs))
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
	rc, err := resolveClass("uartlite", &design.DeviceClass{Entity: "uartlitedb"}, lib)
	if err != nil {
		t.Fatalf("err: %v", err)
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
	if _, err := resolveClass("c", &design.DeviceClass{Entity: "e"}, lib2); !errors.Is(err, ErrAmbiguousArch) {
		t.Errorf("want ambiguous-arch error, got %v", err)
	}
	// zero architectures
	lib0 := buildLib(t, `entity e is end entity;`)
	if _, err := resolveClass("c", &design.DeviceClass{Entity: "e"}, lib0); !errors.Is(err, ErrNoArch) {
		t.Errorf("want no-arch error, got %v", err)
	}
	// unknown entity
	if _, err := resolveClass("c", &design.DeviceClass{Entity: "ghost"}, lib0); !errors.Is(err, ErrEntityNotFound) {
		t.Errorf("want entity error, got %v", err)
	}
}

func TestResolveClassConfiguration(t *testing.T) {
	lib := buildLib(t,
		`entity cpu is end entity;`,
		`architecture rtl of cpu is begin end architecture;`,
		`configuration cpu_cfg of cpu is for rtl end for; end configuration;`)
	rc, err := resolveClass("c", &design.DeviceClass{Entity: "cpu", Configuration: "cpu_cfg"}, lib)
	if err != nil {
		t.Fatalf("err: %v", err)
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
	rc, err := resolveClass("c", &design.DeviceClass{Entity: "e", Architecture: "a2"}, lib)
	if err != nil {
		t.Fatalf("explicit arch should resolve cleanly: %v", err)
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
	_, err := resolveClass("c", &design.DeviceClass{Entity: "e", Architecture: "other", Configuration: "ecfg"}, lib)
	if !errors.Is(err, ErrArchConfigMismatch) {
		t.Errorf("want arch/config mismatch error, got %v", err)
	}
}

func iptr(i int) *int { return &i }

func TestResolveRegs(t *testing.T) {
	lib := buildLib(t, `entity e is end entity;`, `architecture a of e is begin end architecture;`)
	dc := &design.DeviceClass{Entity: "e", Regs: []*design.Reg{
		{Name: "RX"},                                   // addr 0, width 4 -> [0,3]
		{Name: "tx", Width: iptr(4)},                   // addr 4 -> [4,7]
		{Name: "ctrl", Addr: iptr(12), Width: iptr(4)}, // explicit [12,15]
	}}
	rc, err := resolveClass("uartlite", dc, lib)
	if err != nil {
		t.Fatalf("err: %v", err)
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
	_, err := resolveClass("c", dc, lib)
	if !errors.Is(err, ErrRegisterOverlap) {
		t.Fatalf("want register-overlap error, got %v", err)
	}
}

func TestResolveRegsLeftAddrTooSmall(t *testing.T) {
	lib := buildLib(t, `entity e is end entity;`, `architecture a of e is begin end architecture;`)
	dc := &design.DeviceClass{Entity: "e", LeftAddrBit: 1, Regs: []*design.Reg{
		{Name: "a", Addr: iptr(0), Width: iptr(16)}, // needs left-addr-bit >= ceil(log2 16)-1 = 3
	}}
	_, err := resolveClass("c", dc, lib)
	if !errors.Is(err, ErrLeftAddrBitTooSmall) {
		t.Fatalf("want left-addr-bit-too-small error, got %v", err)
	}
}
