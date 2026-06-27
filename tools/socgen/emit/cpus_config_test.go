package emit

import (
	"strings"
	"testing"

	"github.com/j-core/jcore-soc/tools/socgen/design"
	"github.com/j-core/jcore-soc/tools/socgen/vhdl"
)

func TestCPUsConfigJ4Rom(t *testing.T) {
	name, src, files, err := CPUsConfig(&design.CPU{
		Architecture: "one_cpu_m0", Cores: 1, Model: "j4", Decode: "rom", Cache: "id",
	})
	if err != nil {
		t.Fatal(err)
	}
	if name != "soc_cpus_config" {
		t.Errorf("name = %q, want soc_cpus_config", name)
	}
	for _, want := range []string{
		"configuration soc_cpus_config of cpus is",
		"for one_cpu_m0",
		"for all : cpu_core",
		"use entity work.cpu_core(arch);",
		"for u_cpu : cpu",
		"use configuration work.cpu_synth_j4_rom",
		"generic map (",
		"PRIV_ARCH => true",
		"MMU_ARCH => true",
		"end configuration;",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("generated config missing %q:\n%s", want, src)
		}
	}
	if len(files) == 0 {
		t.Error("expected synth filelist additions")
	}
}

func TestCPUsConfigJ2NoGenericMap(t *testing.T) {
	_, src, _, err := CPUsConfig(&design.CPU{
		Architecture: "one_cpu_m0", Cores: 1, Model: "j2", Decode: "direct", Cache: "id",
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(src, "generic map") {
		t.Errorf("J2 binding must not emit a generic map:\n%s", src)
	}
	if !strings.Contains(src, "use configuration work.cpu_synth_direct;") {
		t.Errorf("J2 must bind cpu_synth_direct:\n%s", src)
	}
}

func TestCPUsConfigReparses(t *testing.T) {
	_, src, _, err := CPUsConfig(&design.CPU{
		Architecture: "one_cpu_m0", Cores: 1, Model: "j2", Decode: "direct", Cache: "id",
	})
	if err != nil {
		t.Fatal(err)
	}
	// Wrap with library/use context then re-parse.
	full := "library ieee;\nuse ieee.std_logic_1164.all;\n" + src
	if _, perr := vhdl.ParseFile(vhdl.NewFileSet(), "cpus_config.vhd", []byte(full)); perr != nil {
		t.Fatalf("generated cpus config does not re-parse: %v\n%s", perr, src)
	}
}
