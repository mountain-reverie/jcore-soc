package elaborate

import "fmt"

// CPUsConfigName is the stable name of the generated cpus configuration
// declaration. Defined here (not in emit) so that elaborate can reference it
// without creating an import cycle (emit already imports elaborate).
const CPUsConfigName = "soc_cpus_config"

// cpuSynth maps (model, decode, mult) to the cpu repo's synth configuration
// name and the model/decode-specific source files the synth filelist needs.
// These are soc-side IMPLEMENTATION choices only (which decode table variant
// -- ROM vs direct decoder -- and which multiplier, e.g. ice40 DSP): they are
// not architectural facts, so they stay hardcoded here rather than in
// components/cpu/variants.toml. Paths are cpu-submodule-relative (no
// components/cpu/ prefix); the consumer (filelist.sh) prefixes them with $CPU/.
// These files are the variant-specific decode tables, their configurations, the
// cpu_synth config, and the alternate register/mult/shifter architectures the
// binding selects; they are appended after decode_core (the configuration picks
// among coexisting architectures), exactly as components/cpu/synth/cpu_synth.sh
// appends them. Verified by ghdl analysis of each (model, decode, mult) set.
//
// The ARCHITECTURAL facts -- the generics a variant binds true (e.g. j4's
// PRIV_ARCH, which now implies MMU: the submodule dropped the separate
// MMU_ARCH generic) -- come from components/cpu/variants.toml via
// variantFor, composed in with these files by CPUSynthConfig below.
//
// NOTE: core/tlb.vhd is NOT listed here, even though variants.toml's [j4]
// table names it in extra_files. cpu.vhd directly instantiates work.tlb
// (entity instantiation inside a `g_mmu : if PRIV_ARCH generate`), so ghdl
// requires tlb analyzed BEFORE cpu.vhd regardless of variant. It therefore
// belongs in the static base list ahead of cpu.vhd (matching cpu_synth.sh's
// base FILES), not in this post-decode_core fragment. filelist.sh carries it
// in the base array unconditionally; CPUSynthConfig deliberately does not
// re-emit it here (see cpuVariantExtraFiles below).
// defaultCPUROMWidth MUST match components/cpu/Makefile.inc's own
// CPU_ROM_WIDTH default (currently 72; not surfaced anywhere in
// design.yaml/socgen today, so there is only ever one value in practice).
// It is part of decodeGenDir's path because Makefile.inc's DECODE_GEN_DIR
// is keyed by CPU_VARIANT AND CPU_ROM_WIDTH together (gen/<variant>-w<width>/) --
// two consumers of one variant at different widths must not share a
// directory, the same staleness class the per-variant key exists to
// prevent one level up. If components/cpu/Makefile.inc's default ever
// changes, this must change with it.
const defaultCPUROMWidth = "72"

// decodeGenDir returns this model's out-of-tree cpugen output directory,
// cpu-submodule-relative: components/cpu/Makefile.inc's DECODE_GEN_DIR
// default is $(CPU_INC_DIR)gen/$(CPU_VARIANT)-w$(CPU_ROM_WIDTH), so from the
// submodule root that is gen/<model>-w<width>/decode/.
func decodeGenDir(model string) string {
	return "gen/" + model + "-w" + defaultCPUROMWidth + "/decode"
}

// decodeGenFiles returns the THREE model-invariant cpugen outputs --
// decode_pkg.vhd, decode.vhd, decode_body.vhd -- from model's generated
// directory. ALL SIX cpugen outputs (these three plus the model's selected
// decode_table_<kind>.vhd) are emitted by ONE cpugen invocation and share
// layout constants (e.g. DEC_ADDR_BITS in decode_pkg.vhd must match the
// table geometry in decode_table_<kind>.vhd) -- so a consumer that takes
// some of the six from the committed base decode/ tree and others from
// gen/<model>/decode/ ends up with an internally-inconsistent decoder (this
// is exactly the bug a prior round of review found: j4-rom's cpu_synth_files
// pointed decode_table_rom.vhd at gen/j4/decode/ while filelist.sh still
// hardcoded the BASE decode_pkg.vhd/decode.vhd/decode_body.vhd -- two
// different table geometries wired to the same decoder shell). Callers of
// CPUSynthConfig/CPUSynthConfigFPGAOpt get these three unconditionally, for
// every model (including j2/j1, where gen/<model>/decode/ byte-matches the
// committed base per Task 5's report -- so this is a no-op change in
// content, only in path, for models with no overlay).
func decodeGenFiles(model string) []string {
	dir := decodeGenDir(model)
	return []string{dir + "/decode_pkg.vhd", dir + "/decode.vhd", dir + "/decode_body.vhd"}
}

// decodeTableFile returns the model's selected decode_table_<kind>.vhd,
// from the SAME gen/<model>/decode/ directory as decodeGenFiles -- see its
// comment for why splitting the six cpugen outputs across two source trees
// is the bug this function (and decodeGenFiles) exists to prevent.
func decodeTableFile(model, kind string) string {
	return decodeGenDir(model) + "/decode_table_" + kind + ".vhd"
}

var cpuSynth = map[[3]string]struct {
	cfg   string
	files []string
}{
	{"j2", "direct", ""}: {"cpu_synth_direct", []string{"decode/decode_table_direct_config.vhd", "synth/cpu_synth_config.vhd"}},
	{"j1", "rom", ""}:    {"cpu_synth_j1", []string{"core/register_file_ebr.vhd", "core/mult_seq.vhd", "core/shifter_seq.vhd", "decode/decode_table_rom_config.vhd", "synth/cpu_synth_j1_config.vhd"}},
	// core/dsp_arith.vhd: single-SB_MAC16 DSP-backed ALU adder, enabled only
	// for this variant via synth/cpu_synth_j1_dsp_config.vhd (DSP_ALU=>true).
	// Must be analyzed here even though it is unused hardware for J2/J4/other
	// J1 variants (their datapath keeps DSP_ALU=false -> the arith_unit LUT fn).
	{"j1", "rom", "dsp"}: {"cpu_synth_j1_dsp", []string{"core/register_file_ebr.vhd", "core/mult_ice40dsp.vhd", "core/dsp_arith.vhd", "core/shifter_seq.vhd", "decode/decode_table_rom_config.vhd", "synth/cpu_synth_j1_dsp_config.vhd"}},
	// j4's decode_table_{direct,rom}.vhd come from gen/j4/decode/ -- the
	// out-of-tree sh4-overlay regeneration (components/cpu/Makefile.inc's
	// CPU_DECODE_GENERATED, DECODE_GEN_DIR default $(CPU_INC_DIR)gen/$(CPU_VARIANT)) --
	// NOT the committed decode/ tree. The committed decode/decode_table_*.vhd
	// are the BASE (j2, no-overlay) generation: LDTLB and the PTEH/PTEL/ASIDR
	// LDC/STC family decode as General Illegal there. j4 boards previously
	// built PRIV_ARCH RTL (TLB, MMU CSRs, banked regfile) against that base
	// decoder, so the MMU was present but unreachable from software.
	// decode_table_*_config.vhd (the CONFIGURATION wrapper that binds the
	// table architecture by name) is NOT cpugen-generated -- same file for
	// every variant -- so it still points at the committed decode/ tree.
	{"j4", "direct", ""}: {"cpu_synth_j4", []string{"decode/decode_table_direct_config.vhd", "synth/cpu_synth_j4_config.vhd"}},
	{"j4", "rom", ""}:    {"cpu_synth_j4_rom", []string{"decode/decode_table_rom_config.vhd", "synth/cpu_synth_j4_rom_config.vhd"}},
}

// cpuVariantExtraFiles filters a variant's variants.toml extra_files down to
// the subset CPUSynthConfig should append to its post-decode_core fragment.
// core/tlb.vhd is excluded: jcore-soc's filelist.sh already carries it in the
// static base array (ahead of cpu.vhd, for every model), so re-emitting it
// here would analyze it twice.
func cpuVariantExtraFiles(v cpuVariant) []string {
	var out []string
	for _, f := range v.ExtraFiles {
		if f == "core/tlb.vhd" {
			continue
		}
		out = append(out, f)
	}
	return out
}

// CPUSynthConfig returns the cpu_synth configuration name, the generic map to
// bind it with, and extra filelist sources for a (model, decode, mult) triple.
// mult is "" for the model's native multiplier, or "dsp" for mult(ice40dsp).
// The generics and any architectural extra files come from
// components/cpu/variants.toml (the submodule's single authoritative variant
// table); the config name and decode/mult-specific files come from the
// soc-side cpuSynth table above.
func CPUSynthConfig(model, decode, mult string) (string, map[string]string, []string, error) {
	e, ok := cpuSynth[[3]string{model, decode, mult}]
	if !ok {
		return "", nil, nil, fmt.Errorf("unsupported cpu model/decode/mult combination %q/%q/%q", model, decode, mult)
	}
	v, err := variantFor(model)
	if err != nil {
		return "", nil, nil, err
	}
	// decodeGenFiles + decodeTableFile: all four model-varying cpugen outputs
	// this (model, decode) combo needs, from the SAME gen/<model>/decode/
	// directory -- see decodeGenFiles' comment for why splitting them across
	// two source trees is unsafe.
	files := append(cpuVariantExtraFiles(v), decodeGenFiles(model)...)
	files = append(files, decodeTableFile(model, decode))
	files = append(files, e.files...)
	return e.cfg, v.Generics, files, nil
}

// cpuSynthFPGAOpt maps (model, decode, mult) to the FPGA-optimised cpu_synth
// configuration -- same core, but with the register file in ECP5 block RAM
// (register_file(ebr)) instead of ~1.4k LUT4 of distributed RAM. Used for the
// FPGA-optimised core (core0) of an asymmetric dual; the peer core keeps the
// standard (portable / ASIC-representative) config from cpuSynth. The extra
// files are the ebr regfile arch + the *_ebr synth config; the shared decode
// tables come from the standard config's files (analyzed once).
var cpuSynthFPGAOpt = map[[3]string]struct {
	cfg   string
	files []string
}{
	{"j4", "rom", ""}: {"cpu_synth_j4_rom_ebr",
		[]string{"core/register_file_ebr.vhd", "synth/cpu_synth_j4_rom_ebr_config.vhd"}},
}

// CPUSynthConfigFPGAOpt returns the FPGA-optimised cpu_synth config for a
// (model, decode, mult) triple, or an error if no FPGA-optimised variant exists.
// Generics come from components/cpu/variants.toml, same as CPUSynthConfig.
func CPUSynthConfigFPGAOpt(model, decode, mult string) (string, map[string]string, []string, error) {
	e, ok := cpuSynthFPGAOpt[[3]string{model, decode, mult}]
	if !ok {
		return "", nil, nil, fmt.Errorf("no FPGA-optimised cpu variant for model/decode/mult %q/%q/%q", model, decode, mult)
	}
	v, err := variantFor(model)
	if err != nil {
		return "", nil, nil, err
	}
	files := append(cpuVariantExtraFiles(v), e.files...)
	return e.cfg, v.Generics, files, nil
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
