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
		df, errs := vhdl.ParseFile(vhdl.NewFileSet(), "t.vhd", []byte(s))
		if len(errs) != 0 {
			t.Fatalf("parse src %d: %v", i, errs)
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
