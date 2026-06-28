package elaborate

import (
	"strings"
	"testing"
)

func TestCPUSynthConfig(t *testing.T) {
	cases := []struct {
		model, decode, want string
		priv                bool
		wantFiles           []string // must all be present in files (subset check)
	}{
		{"j2", "direct", "cpu_synth_direct", false, []string{
			"decode/decode_table_direct.vhd", "decode/decode_table_direct_config.vhd", "synth/cpu_synth_config.vhd"}},
		{"j1", "rom", "cpu_synth_j1", false, []string{
			"core/register_file_ebr.vhd", "core/mult_seq.vhd", "core/shifter_seq.vhd", "decode/decode_table_rom.vhd", "decode/decode_table_rom_config.vhd", "synth/cpu_synth_j1_config.vhd"}},
		{"j4", "direct", "cpu_synth_j4", true, []string{
			"decode/decode_table_direct.vhd", "decode/decode_table_direct_config.vhd", "synth/cpu_synth_j4_config.vhd"}},
		{"j4", "rom", "cpu_synth_j4_rom", true, []string{
			"decode/decode_table_rom.vhd", "decode/decode_table_rom_config.vhd", "synth/cpu_synth_j4_rom_config.vhd"}},
	}
	for _, c := range cases {
		cfg, gen, files, err := CPUSynthConfig(c.model, c.decode)
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
			found := false
			for _, f := range files {
				if f == want {
					found = true
				}
			}
			if !found {
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
	if _, _, _, err := CPUSynthConfig("j9", "direct"); err == nil {
		t.Error("unknown model must error")
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
