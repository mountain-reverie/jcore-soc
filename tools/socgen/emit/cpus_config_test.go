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
	for _, want := range []string{
		"synth/cpu_synth_j4_rom_config.vhd", "decode/decode_table_rom.vhd", "decode/decode_table_rom_config.vhd",
	} {
		found := false
		for _, f := range files {
			if f == want {
				found = true
			}
		}
		if !found {
			t.Errorf("j4-rom synth files missing %q; got %v", want, files)
		}
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

func TestCPUsConfigAsymmetricEBR(t *testing.T) {
	// fpga-opt-core0: core0 binds the FPGA-optimised (ebr) config, core1 the
	// standard config, each with a distinct CORE_ID so the two cpu instances get
	// distinct ghdl->yosys module names.
	_, src, files, err := CPUsConfig(&design.CPU{
		Architecture: "two_cpu_m0", Cores: 2, Model: "j4", Decode: "rom", Cache: "id",
		FpgaOptCore0: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"for core0 : cpu_core",
		"for core1 : cpu_core",
		"use configuration work.cpu_synth_j4_rom_ebr",
		"use configuration work.cpu_synth_j4_rom\n", // core1 standard (not _ebr)
		"CORE_ID => 0",
		"CORE_ID => 1",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("asymmetric config missing %q:\n%s", want, src)
		}
	}
	if strings.Contains(src, "for all : cpu_core") {
		t.Errorf("asymmetric config must be per-core, not `for all`:\n%s", src)
	}
	// ebr regfile + its synth config must be in the filelist.
	for _, want := range []string{"core/register_file_ebr.vhd", "synth/cpu_synth_j4_rom_ebr_config.vhd"} {
		found := false
		for _, f := range files {
			if f == want {
				found = true
			}
		}
		if !found {
			t.Errorf("asymmetric filelist missing %q; got %v", want, files)
		}
	}
}

func TestCPUsConfigAsymmetricRequiresTwoCores(t *testing.T) {
	_, _, _, err := CPUsConfig(&design.CPU{
		Architecture: "one_cpu_m0", Cores: 1, Model: "j4", Decode: "rom", Cache: "id",
		FpgaOptCore0: true,
	})
	if err == nil {
		t.Errorf("fpga-opt-core0 with cores=1 must error")
	}
}
