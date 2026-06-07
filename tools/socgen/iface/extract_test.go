package iface

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/j-core/jcore-soc/tools/socgen/vhdl"
)

func parse(t *testing.T, src string) *vhdl.DesignFile {
	t.Helper()
	df, err := vhdl.ParseFile(vhdl.NewFileSet(), "t.vhd", []byte(src))
	if err != nil {
		t.Fatalf("parse errors: %v", err)
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

func TestExtractPackage(t *testing.T) {
	df := parse(t, `package bus_pkg is
  constant WIDTH : integer := 32;
  constant DEFERRED : integer;
  subtype byte is std_logic_vector(7 downto 0);
  type state_t is (idle, run, stop);
  component fifo is
    generic (depth : integer);
    port (clk : in std_logic; full : out std_logic);
  end component;
end package;`)
	lib, errs := Extract([]*vhdl.DesignFile{df})
	if len(errs) != 0 {
		t.Fatalf("extract errors: %v", errs)
	}
	p, ok := lib.Package("bus_pkg")
	if !ok {
		t.Fatal("package bus_pkg not found")
	}
	if len(p.Constants) != 2 {
		t.Fatalf("constants: got %d want 2", len(p.Constants))
	}
	if p.Constants[0].Name != "WIDTH" || p.Constants[0].Type.String() != "integer" || p.Constants[0].Value == nil {
		t.Errorf("const0 = %+v", p.Constants[0])
	}
	if p.Constants[1].Name != "DEFERRED" || p.Constants[1].Value != nil {
		t.Errorf("const1 should be deferred: %+v", p.Constants[1])
	}
	if len(p.Types) != 2 { // subtype byte + type state_t
		t.Fatalf("types: got %d want 2", len(p.Types))
	}
	if len(p.Components) != 1 || p.Components[0].Name != "fifo" {
		t.Fatalf("components: %+v", p.Components)
	}
	if len(p.Components[0].Ports) != 2 || p.Components[0].Ports[1].Name != "full" || p.Components[0].Ports[1].Dir != "out" {
		t.Errorf("component ports: %+v", p.Components[0].Ports)
	}
	if te, ok := lib.ResolveType("byte"); !ok || te.Name != "byte" {
		t.Errorf("ResolveType(byte) = %v %v", te, ok)
	}
	if te, ok := lib.ResolveType("state_t"); !ok || te.Name != "state_t" {
		t.Errorf("ResolveType(state_t) = %v %v", te, ok)
	}
}

func TestExtractConfiguration(t *testing.T) {
	df := parse(t, `configuration cfg of uart is
  for rtl
  end for;
end configuration;`)
	lib, errs := Extract([]*vhdl.DesignFile{df})
	if len(errs) != 0 {
		t.Fatalf("extract errors: %v", errs)
	}
	c, ok := lib.Configuration("cfg")
	if !ok {
		t.Fatal("configuration cfg not found")
	}
	if c.Entity != "uart" || c.Arch != "rtl" || c.Node == nil {
		t.Errorf("config = %+v", c)
	}
}

func TestExtractDuplicateEntity(t *testing.T) {
	a := parse(t, `entity dup is end entity;`)
	b := parse(t, `entity dup is end entity;`)
	_, errs := Extract([]*vhdl.DesignFile{a, b})
	if len(errs) == 0 {
		t.Fatal("expected a duplicate-entity error")
	}
}

func TestExtractEmpty(t *testing.T) {
	lib, errs := Extract(nil)
	if lib == nil || len(errs) != 0 {
		t.Fatalf("empty input: lib=%v errs=%v", lib, errs)
	}
	if _, ok := lib.Entity("nope"); ok {
		t.Error("empty lib should have no entities")
	}
}

// Carry-forward from T2 review: negative ResolveType + component generic assertion.
func TestResolveTypeNegative(t *testing.T) {
	df := parse(t, `package p is
  constant C : integer := 1;
  component comp is port (x : in std_logic); end component;
end package;`)
	lib, _ := Extract([]*vhdl.DesignFile{df})
	if _, ok := lib.ResolveType("C"); ok {
		t.Error("ResolveType should return false for a constant name")
	}
	if _, ok := lib.ResolveType("comp"); ok {
		t.Error("ResolveType should return false for a component name")
	}
	if _, ok := lib.ResolveType("nonexistent"); ok {
		t.Error("ResolveType should return false for an unknown name")
	}
}

func TestExtractComponentGeneric(t *testing.T) {
	df := parse(t, `package p is
  component fifo is generic (depth : integer); port (clk : in std_logic); end component;
end package;`)
	lib, _ := Extract([]*vhdl.DesignFile{df})
	p, _ := lib.Package("p")
	if len(p.Components[0].Generics) != 1 || p.Components[0].Generics[0].Name != "depth" {
		t.Errorf("component generics: %+v", p.Components[0].Generics)
	}
}

func TestExtractCorpusSmoke(t *testing.T) {
	root := os.Getenv("JCORE_SOC_ROOT")
	if root == "" {
		t.Skip("JCORE_SOC_ROOT not set")
	}
	rels := []string{
		"components/cpu/cpu2j0_pkg.vhd",
		"components/uartlite/uartlitedb.vhd",
	}
	var files []*vhdl.DesignFile
	for _, rel := range rels {
		src, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			t.Skipf("corpus file missing: %s", rel)
		}
		df, perr := vhdl.ParseFile(vhdl.NewFileSet(), rel, src)
		if perr != nil {
			t.Skipf("parse %s: %v", rel, perr)
		}
		files = append(files, df)
	}
	lib, errs := Extract(files)
	for _, e := range errs {
		t.Logf("extract note: %v", e)
	}
	if len(lib.Packages) == 0 {
		t.Error("expected at least one package")
	}
	if len(lib.Entities) == 0 {
		t.Error("expected at least one entity")
	}
	// concrete check: cpu2j0_pkg declares type cpu_data_o_t (verified via grep).
	if _, ok := lib.ResolveType("cpu_data_o_t"); !ok {
		t.Errorf("expected cpu2j0_pkg to declare type cpu_data_o_t")
	}
}

func TestExtractDuplicatePackage(t *testing.T) {
	a := parse(t, `package dup is end package;`)
	b := parse(t, `package dup is end package;`)
	_, errs := Extract([]*vhdl.DesignFile{a, b})
	if len(errs) == 0 {
		t.Fatal("expected a duplicate-package error")
	}
}

func TestExtractDuplicateConfiguration(t *testing.T) {
	a := parse(t, `configuration c of e is for rtl end for; end configuration;`)
	b := parse(t, `configuration c of e is for rtl end for; end configuration;`)
	_, errs := Extract([]*vhdl.DesignFile{a, b})
	if len(errs) == 0 {
		t.Fatal("expected a duplicate-configuration error")
	}
}

func TestExtractDuplicateSymbol(t *testing.T) {
	a := parse(t, `package p1 is constant SYM_COMMON : integer := 1; end package;`)
	b := parse(t, `package p2 is constant SYM_COMMON : integer := 2; end package;`)
	_, errs := Extract([]*vhdl.DesignFile{a, b})
	if len(errs) == 0 {
		t.Fatal("expected a duplicate-symbol error (SYM_COMMON in p1 and p2)")
	}
}
