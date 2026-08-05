package elaborate

import (
	"slices"
	"strings"
	"testing"
)

func TestCPUSynthConfig(t *testing.T) {
	cases := []struct {
		model, decode, want string
		priv                bool
		wantFiles           []string // must all be present in files (subset check)
	}{
		// All six cpugen outputs must come from the SAME gen/<model>/decode/
		// directory (decode_pkg/decode/decode_body + the selected table) --
		// see decodeGenFiles' comment. True for every model, not just j4:
		// j2/j1 have no overlay, so gen/<model>/decode/ byte-matches the
		// committed base (Task 5), but the PATH must still be uniform so no
		// consumer can accidentally mix a gen/ table with a base decode_pkg.
		{"j2", "direct", "cpu_synth_direct", false, []string{
			"gen/j2-w72/decode/decode_pkg.vhd", "gen/j2-w72/decode/decode.vhd", "gen/j2-w72/decode/decode_body.vhd",
			"gen/j2-w72/decode/decode_table_direct.vhd", "decode/decode_table_direct_config.vhd", "synth/cpu_synth_config.vhd"}},
		{"j1", "rom", "cpu_synth_j1", false, []string{
			"gen/j1-w72/decode/decode_pkg.vhd", "gen/j1-w72/decode/decode.vhd", "gen/j1-w72/decode/decode_body.vhd",
			"core/register_file_ebr.vhd", "core/mult_seq.vhd", "core/shifter_seq.vhd",
			"gen/j1-w72/decode/decode_table_rom.vhd", "decode/decode_table_rom_config.vhd", "synth/cpu_synth_j1_config.vhd"}},
		{"j4", "direct", "cpu_synth_j4", true, []string{
			"gen/j4-w72/decode/decode_pkg.vhd", "gen/j4-w72/decode/decode.vhd", "gen/j4-w72/decode/decode_body.vhd",
			"gen/j4-w72/decode/decode_table_direct.vhd", "decode/decode_table_direct_config.vhd", "synth/cpu_synth_j4_config.vhd"}},
		{"j4", "rom", "cpu_synth_j4_rom", true, []string{
			"gen/j4-w72/decode/decode_pkg.vhd", "gen/j4-w72/decode/decode.vhd", "gen/j4-w72/decode/decode_body.vhd",
			"gen/j4-w72/decode/decode_table_rom.vhd", "decode/decode_table_rom_config.vhd", "synth/cpu_synth_j4_rom_config.vhd"}},
	}
	for _, c := range cases {
		cfg, gen, files, err := CPUSynthConfig(c.model, c.decode, "")
		if err != nil {
			t.Fatalf("%s/%s: %v", c.model, c.decode, err)
		}
		if cfg != c.want {
			t.Errorf("%s/%s: got %q want %q", c.model, c.decode, cfg, c.want)
		}
		if c.priv && gen["PRIV_ARCH"] != "true" {
			t.Errorf("%s/%s: expected PRIV_ARCH=>true, got %v", c.model, c.decode, gen)
		}
		for _, want := range c.wantFiles {
			if !slices.Contains(files, want) {
				t.Errorf("%s/%s: files missing %q; got %v", c.model, c.decode, want, files)
			}
		}
		// tlb is a base file (analyzed before cpu.vhd), never in this fragment.
		for _, f := range files {
			if strings.HasSuffix(f, "tlb.vhd") {
				t.Errorf("%s/%s: tlb.vhd must not be in the post-decode_core fragment; got %v", c.model, c.decode, files)
			}
		}
	}
	if _, _, _, err := CPUSynthConfig("j9", "direct", ""); err == nil {
		t.Error("unknown model must error")
	}
}

func TestCPUSynthConfigDSP(t *testing.T) {
	cfg, generics, files, err := CPUSynthConfig("j1", "rom", "dsp")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg != "cpu_synth_j1_dsp" {
		t.Fatalf("cfg = %q, want cpu_synth_j1_dsp", cfg)
	}
	if generics != nil {
		t.Fatalf("generics = %v, want nil", generics)
	}
	wantFile := func(name string) {
		if !slices.Contains(files, name) {
			t.Fatalf("files %v missing %q", files, name)
		}
	}
	wantFile("core/mult_ice40dsp.vhd")
	wantFile("synth/cpu_synth_j1_dsp_config.vhd")
	if slices.Contains(files, "core/mult_seq.vhd") || slices.Contains(files, "synth/cpu_synth_j1_config.vhd") {
		t.Fatalf("dsp filelist must not contain the seq mult/config: %v", files)
	}

	cfg0, _, files0, err := CPUSynthConfig("j1", "rom", "")
	if err != nil || cfg0 != "cpu_synth_j1" {
		t.Fatalf("native j1: cfg=%q err=%v", cfg0, err)
	}
	if slices.Contains(files0, "core/mult_ice40dsp.vhd") {
		t.Fatalf("native j1 filelist leaked the dsp mult: %v", files0)
	}
}

func TestJ4GenericsComeFromVariantsTOML(t *testing.T) {
	cfg, generics, files, err := CPUSynthConfig("j4", "direct", "")
	if err != nil {
		t.Fatalf("CPUSynthConfig: %v", err)
	}
	if cfg != "cpu_synth_j4" {
		t.Errorf("cfg = %q, want cpu_synth_j4", cfg)
	}
	if generics["PRIV_ARCH"] != "true" {
		t.Errorf("PRIV_ARCH = %q, want true", generics["PRIV_ARCH"])
	}
	if _, bad := generics["MMU_ARCH"]; bad {
		t.Error("MMU_ARCH must be gone: PRIV_ARCH implies MMU")
	}
	// Task 12 wired the deferred piece: j4 now points at the sh4-overlay
	// out-of-tree regeneration (gen/j4-w72/decode/...), not the committed base
	// decode_table_direct.vhd. filelist.sh prefixes every entry with $CPU/,
	// so this resolves to $CPU/gen/j4-w72/decode/decode_table_direct.vhd -- the
	// directory components/cpu/Makefile.inc's CPU_DECODE_GENERATED rule
	// (DECODE_GEN_DIR default $(CPU_INC_DIR)gen/$(CPU_VARIANT)) regenerates
	// with the sh4 overlay applied (LDTLB decodes instead of General Illegal).
	if !slices.Contains(files, "gen/j4-w72/decode/decode_table_direct.vhd") {
		t.Errorf("files missing gen/j4-w72/decode/decode_table_direct.vhd (sh4-overlay decoder); got %v", files)
	}
	if slices.Contains(files, "decode/decode_table_direct.vhd") {
		t.Errorf("files must not point at the committed BASE decode_table_direct.vhd; got %v", files)
	}
}

func TestJ2Unchanged(t *testing.T) {
	cfg, generics, _, err := CPUSynthConfig("j2", "direct", "")
	if err != nil {
		t.Fatalf("CPUSynthConfig: %v", err)
	}
	if cfg != "cpu_synth_direct" || len(generics) != 0 {
		t.Errorf("j2 = %q/%v, want cpu_synth_direct/empty", cfg, generics)
	}
}

func TestRAMMuxConfig(t *testing.T) {
	cfg, _, err := RAMMuxConfig(1, "id")
	if err != nil || cfg != "ddr_ram_mux_one_cpu_idcache_fpga" {
		t.Fatalf("1/id: got %q err %v", cfg, err)
	}
	if _, _, err := RAMMuxConfig(1, "none"); err != nil {
		t.Errorf("1/none: %v", err)
	}
	if _, _, err := RAMMuxConfig(1, "bogus"); err == nil {
		t.Error("unknown cache must error")
	}
}
