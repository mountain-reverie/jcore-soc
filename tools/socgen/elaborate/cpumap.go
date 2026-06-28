package elaborate

import "fmt"

// CPUsConfigName is the stable name of the generated cpus configuration
// declaration. Defined here (not in emit) so that elaborate can reference it
// without creating an import cycle (emit already imports elaborate).
const CPUsConfigName = "soc_cpus_config"

// cpuSynth maps (model, decode) to the cpu repo's synth configuration name,
// the generics it must be bound with, and the model/decode-specific source
// files the synth filelist needs. Paths are cpu-submodule-relative (no
// components/cpu/ prefix); the consumer (filelist.sh) prefixes them with $CPU/.
// These files are the variant-specific decode tables, their configurations, the
// cpu_synth config, and the alternate register/mult/shifter architectures the
// binding selects; they are appended after decode_core (the configuration picks
// among coexisting architectures), exactly as components/cpu/synth/cpu_synth.sh
// appends them. Verified by ghdl analysis of each (model, decode) set.
//
// NOTE: core/tlb.vhd is NOT listed here. cpu.vhd directly instantiates
// work.tlb (entity instantiation inside a `g_mmu : if MMU_ARCH generate`), so
// ghdl requires tlb analyzed BEFORE cpu.vhd regardless of variant. It therefore
// belongs in the static base list ahead of cpu.vhd (matching cpu_synth.sh's
// base FILES), not in this post-decode_core fragment. filelist.sh carries it in
// the base array.
var cpuSynth = map[[2]string]struct {
	cfg      string
	generics map[string]string
	files    []string
}{
	{"j2", "direct"}: {"cpu_synth_direct", nil, []string{"decode/decode_table_direct.vhd", "decode/decode_table_direct_config.vhd", "synth/cpu_synth_config.vhd"}},
	{"j1", "rom"}:    {"cpu_synth_j1", nil, []string{"core/register_file_ebr.vhd", "core/mult_seq.vhd", "core/shifter_seq.vhd", "decode/decode_table_rom.vhd", "decode/decode_table_rom_config.vhd", "synth/cpu_synth_j1_config.vhd"}},
	{"j4", "direct"}: {"cpu_synth_j4", map[string]string{"PRIV_ARCH": "true", "MMU_ARCH": "true"}, []string{"decode/decode_table_direct.vhd", "decode/decode_table_direct_config.vhd", "synth/cpu_synth_j4_config.vhd"}},
	{"j4", "rom"}:    {"cpu_synth_j4_rom", map[string]string{"PRIV_ARCH": "true", "MMU_ARCH": "true"}, []string{"decode/decode_table_rom.vhd", "decode/decode_table_rom_config.vhd", "synth/cpu_synth_j4_rom_config.vhd"}},
}

// CPUSynthConfig returns the cpu_synth configuration name, the generic map to
// bind it with, and extra filelist sources for a (model, decode) pair.
func CPUSynthConfig(model, decode string) (string, map[string]string, []string, error) {
	e, ok := cpuSynth[[2]string{model, decode}]
	if !ok {
		return "", nil, nil, fmt.Errorf("unsupported cpu model/decode combination %q/%q", model, decode)
	}
	return e.cfg, e.generics, e.files, nil
}

var ramMux = map[[2]any]struct {
	cfg   string
	files []string
}{
	{1, "none"}: {"ddr_ram_mux_one_cpu_direct_fpga", []string{"ddr_ram_mux/one_cpu_direct.vhd"}},
	{1, "i"}:    {"ddr_ram_mux_one_cpu_icache_fpga", []string{"ddr_ram_mux/one_cpu_icache.vhd", "ddr_ram_mux/one_cpu_icache_fpga.vhd"}},
	{1, "id"}:   {"ddr_ram_mux_one_cpu_idcache_fpga", []string{"ddr_ram_mux/one_cpu_idcache.vhd", "ddr_ram_mux/one_cpu_idcache_fpga.vhd"}},
	{2, "id"}:   {"ddr_ram_mux_two_cpu_idcache_fpga", []string{"ddr_ram_mux/two_cpu_idcache.vhd", "ddr_ram_mux/two_cpu_idcache_fpga.vhd"}},
}

// RAMMuxConfig maps (core count, cache level) to the ddr_ram_mux configuration
// name and its source files.
func RAMMuxConfig(cores int, cache string) (string, []string, error) {
	e, ok := ramMux[[2]any{cores, cache}]
	if !ok {
		return "", nil, fmt.Errorf("unsupported cache %q for %d core(s)", cache, cores)
	}
	return e.cfg, e.files, nil
}
