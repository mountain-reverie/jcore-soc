package design

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/j-core/jcore-soc/tools/socgen/iface"
	"github.com/j-core/jcore-soc/tools/socgen/vhdl"
)

// buildLib parses VHDL sources and extracts an iface.Library.
func buildLib(t *testing.T, srcs ...string) *iface.Library {
	t.Helper()
	var files []*vhdl.DesignFile
	for i, s := range srcs {
		df, errs := vhdl.ParseFile(vhdl.NewFileSet(), "t.vhd", []byte(s))
		if len(errs) != 0 {
			t.Fatalf("parse src %d: %v", i, errs)
		}
		files = append(files, df)
	}
	lib, _ := iface.Extract(files)
	return lib
}

func TestValidateOK(t *testing.T) {
	lib := buildLib(t, `entity uartlitedb is
  generic (fclk : integer);
  port (clk : in std_logic; rx : in std_logic);
end entity;`)
	d := &Design{
		DeviceClasses: map[string]*DeviceClass{
			"uartlite": {Entity: "uartlitedb"},
		},
		Devices: []*Device{
			{Class: "uartlite", Name: "uart0",
				Generics: map[string]Value{"fclk": {Kind: KindExpr, Text: "CFG"}},
				Ports:    map[string]Value{"clk": {Kind: KindExpr, Text: "clk_sys"}}},
		},
	}
	if errs := Validate(d, lib); len(errs) != 0 {
		t.Fatalf("expected no errors, got: %v", errs)
	}
}

func TestValidateCatches(t *testing.T) {
	lib := buildLib(t, `entity uartlitedb is
  generic (fclk : integer);
  port (clk : in std_logic);
end entity;`)
	d := &Design{
		DeviceClasses: map[string]*DeviceClass{"uartlite": {Entity: "uartlitedb"}},
		Devices: []*Device{
			{Class: "nope", Name: "d0"},                                              // unknown class
			{Class: "uartlite", Name: "u0", Generics: map[string]Value{"bogus": {}}}, // bad generic
			{Class: "uartlite", Name: "u1", Ports: map[string]Value{"missing": {}}},  // bad port
		},
		TopEntities: map[string]*TopEntity{"t0": {Entity: "ghost_entity"}}, // missing entity
	}
	errs := Validate(d, lib)
	joined := errsToString(errs)
	for _, want := range []string{"unknown class", "generic \"bogus\"", "port \"missing\"", "entity \"ghost_entity\" not found"} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing error %q in:\n%s", want, joined)
		}
	}
}

func errsToString(errs []error) string {
	var b strings.Builder
	for _, e := range errs {
		b.WriteString(e.Error())
		b.WriteByte('\n')
	}
	return b.String()
}

func TestValidateConfiguration(t *testing.T) {
	lib := buildLib(t,
		`entity cpu is port (clk : in std_logic); end entity;`,
		`architecture rtl of cpu is begin end architecture;`,
		`configuration cpu_cfg of cpu is for rtl end for; end configuration;`,
	)
	// happy path: class resolves entity via configuration; arch rtl exists.
	ok := &Design{
		DeviceClasses: map[string]*DeviceClass{"c": {Configuration: "cpu_cfg"}},
		Devices:       []*Device{{Class: "c", Name: "cpu0", Ports: map[string]Value{"clk": {Kind: KindExpr, Text: "clk_sys"}}}},
	}
	if errs := Validate(ok, lib); len(errs) != 0 {
		t.Fatalf("config happy path should pass: %v", errs)
	}
	// missing configuration:
	bad := &Design{
		DeviceClasses: map[string]*DeviceClass{"c": {Configuration: "ghost_cfg"}},
		Devices:       []*Device{{Class: "c", Name: "d0"}},
	}
	if errsToString(Validate(bad, lib)) == "" {
		t.Error("missing configuration should error")
	}
}

func TestValidateAgainstCorpus(t *testing.T) {
	root := os.Getenv("JCORE_SOC_ROOT")
	if root == "" {
		t.Skip("JCORE_SOC_ROOT not set")
	}
	// A handful of real device-class entity/package files (skip cpp-dependent ones).
	rels := []string{
		"components/uartlite/uartlitedb.vhd",
		"components/cpu/cpu2j0_pkg.vhd",
	}
	var files []*vhdl.DesignFile
	for _, rel := range rels {
		src, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			t.Skipf("missing %s", rel)
		}
		df, errs := vhdl.ParseFile(vhdl.NewFileSet(), rel, src)
		if len(errs) != 0 {
			t.Skipf("parse %s: %v", rel, errs)
		}
		files = append(files, df)
	}
	lib, _ := iface.Extract(files)
	// A spec whose uartlite class maps to the real uartlitedb entity.
	// No generics/ports -> only entity-resolution is checked (robust to exact interface).
	d := &Design{
		DeviceClasses: map[string]*DeviceClass{"uartlite": {Entity: "uartlitedb"}},
		Devices:       []*Device{{Class: "uartlite", Name: "uart0"}},
	}
	if errs := Validate(d, lib); len(errs) != 0 {
		t.Fatalf("uartlitedb should resolve from corpus; got: %v", errs)
	}
}
