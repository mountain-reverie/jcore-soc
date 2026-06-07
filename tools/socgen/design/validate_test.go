package design

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/j-core/jcore-soc/tools/socgen/iface"
	"github.com/j-core/jcore-soc/tools/socgen/internal/errutil"
	"github.com/j-core/jcore-soc/tools/socgen/vhdl"
)

// buildLib parses VHDL sources and extracts an iface.Library.
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
	if err := Validate(d, lib); err != nil {
		t.Fatalf("expected no errors, got: %v", err)
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
	err := Validate(d, lib)
	for _, want := range []error{ErrUnknownClass, ErrGenericNotOnEntity, ErrPortNotOnEntity, ErrEntityNotFound} {
		if !errors.Is(err, want) {
			t.Errorf("missing error %v in: %v", want, err)
		}
	}
	// field-level checks via errors.As over the flattened leaves.
	want := map[error]struct{ name, entity string }{
		ErrGenericNotOnEntity: {"bogus", "uartlitedb"},
		ErrPortNotOnEntity:    {"missing", "uartlitedb"},
		ErrEntityNotFound:     {"ghost_entity", ""},
		ErrUnknownClass:       {"nope", ""},
	}
	for _, leaf := range errutil.Errors(err) {
		var ve *ValidateError
		if !errors.As(leaf, &ve) {
			t.Errorf("non-ValidateError leaf: %v", leaf)
			continue
		}
		if w, ok := want[ve.Kind]; ok {
			if ve.Name != w.name || ve.Entity != w.entity {
				t.Errorf("%v: name=%q entity=%q, want name=%q entity=%q", ve.Kind, ve.Name, ve.Entity, w.name, w.entity)
			}
		}
	}
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
	if err := Validate(ok, lib); err != nil {
		t.Fatalf("config happy path should pass: %v", err)
	}
	// missing configuration:
	bad := &Design{
		DeviceClasses: map[string]*DeviceClass{"c": {Configuration: "ghost_cfg"}},
		Devices:       []*Device{{Class: "c", Name: "d0"}},
	}
	if err := Validate(bad, lib); !errors.Is(err, ErrConfigNotFound) {
		t.Errorf("missing configuration should error with ErrConfigNotFound, got %v", err)
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
		df, perr := vhdl.ParseFile(vhdl.NewFileSet(), rel, src)
		if perr != nil {
			t.Skipf("parse %s: %v", rel, perr)
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
	if err := Validate(d, lib); err != nil {
		t.Fatalf("uartlitedb should resolve from corpus; got: %v", err)
	}
}

// TestErrorMessages is a smoke test that the typed errors render their expected
// .Error() strings (message substance preserved from the pre-refactor fmt.Errorf).
func TestErrorMessages(t *testing.T) {
	se := &SpecError{Line: 7, Msg: "invalid address \"zz\"", Err: errors.New("strconv: bad")}
	if got, want := se.Error(), `line 7: invalid address "zz": strconv: bad`; got != want {
		t.Errorf("SpecError = %q, want %q", got, want)
	}
	se2 := &SpecError{Line: 3, Msg: "invalid match node"}
	if got, want := se2.Error(), "line 3: invalid match node"; got != want {
		t.Errorf("SpecError = %q, want %q", got, want)
	}
	ve := &ValidateError{Kind: ErrGenericNotOnEntity, Ctx: "device aic0", Name: "bogus", Entity: "uartlitedb"}
	if got, want := ve.Error(), `device aic0: generic "bogus" not on entity "uartlitedb"`; got != want {
		t.Errorf("ValidateError = %q, want %q", got, want)
	}
	if got, want := (&ValidateError{Kind: ErrUnknownClass, Ctx: `device "d0"`, Name: "nope"}).Error(), `device "d0": unknown class "nope"`; got != want {
		t.Errorf("ValidateError = %q, want %q", got, want)
	}
}
