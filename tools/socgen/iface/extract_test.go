package iface

import (
	"testing"

	"github.com/j-core/jcore-soc/tools/socgen/vhdl"
)

func parse(t *testing.T, src string) *vhdl.DesignFile {
	t.Helper()
	df, errs := vhdl.ParseFile(vhdl.NewFileSet(), "t.vhd", []byte(src))
	if len(errs) != 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	return df
}

func TestExtractEntity(t *testing.T) {
	df := parse(t, `entity uart is
  generic (width : integer := 8; fast : boolean);
  port (clk, rst : in std_logic;
        data : out std_logic_vector(15 downto 0));
end entity;`)
	lib, errs := Extract([]*vhdl.DesignFile{df})
	if len(errs) != 0 {
		t.Fatalf("extract errors: %v", errs)
	}
	e, ok := lib.Entity("uart")
	if !ok {
		t.Fatal("entity uart not found")
	}
	if len(e.Generics) != 2 {
		t.Fatalf("generics: got %d want 2", len(e.Generics))
	}
	if e.Generics[0].Name != "width" || e.Generics[0].Type.String() != "integer" {
		t.Errorf("generic0 = %q %q", e.Generics[0].Name, e.Generics[0].Type.String())
	}
	if e.Generics[0].Default == nil {
		t.Error("generic0 default should be non-nil")
	}
	if e.Generics[1].Name != "fast" || e.Generics[1].Default != nil {
		t.Errorf("generic1 = %q default=%v", e.Generics[1].Name, e.Generics[1].Default)
	}
	if len(e.Ports) != 3 {
		t.Fatalf("ports: got %d want 3", len(e.Ports))
	}
	if e.Ports[0].Name != "clk" || e.Ports[0].Dir != "in" || e.Ports[0].Type.String() != "std_logic" {
		t.Errorf("port0 = %+v (%s)", e.Ports[0], e.Ports[0].Type.String())
	}
	if e.Ports[1].Name != "rst" || e.Ports[1].Dir != "in" {
		t.Errorf("port1 = %+v", e.Ports[1])
	}
	if e.Ports[2].Name != "data" || e.Ports[2].Dir != "out" ||
		e.Ports[2].Type.String() != "std_logic_vector(15 downto 0)" {
		t.Errorf("port2 = %+v (%s)", e.Ports[2], e.Ports[2].Type.String())
	}
}

func TestExtractArchitecture(t *testing.T) {
	df := parse(t, `architecture rtl of uart is begin end architecture;`)
	lib, errs := Extract([]*vhdl.DesignFile{df})
	if len(errs) != 0 {
		t.Fatalf("extract errors: %v", errs)
	}
	archs := lib.ArchitecturesOf("uart")
	if len(archs) != 1 {
		t.Fatalf("architectures of uart: got %d want 1", len(archs))
	}
	if archs[0].Name != "rtl" || archs[0].Entity != "uart" || archs[0].Node == nil {
		t.Errorf("arch = %+v", archs[0])
	}
}
