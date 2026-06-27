package elaborate

import "testing"

func TestCPUSynthConfig(t *testing.T) {
	cases := []struct {
		model, decode, want string
		priv                bool
	}{
		{"j2", "direct", "cpu_synth_direct", false},
		{"j1", "rom", "cpu_synth_j1", false},
		{"j4", "direct", "cpu_synth_j4", true},
		{"j4", "rom", "cpu_synth_j4_rom", true},
	}
	for _, c := range cases {
		cfg, gen, _, err := CPUSynthConfig(c.model, c.decode)
		if err != nil {
			t.Fatalf("%s/%s: %v", c.model, c.decode, err)
		}
		if cfg != c.want {
			t.Errorf("%s/%s: got %q want %q", c.model, c.decode, cfg, c.want)
		}
		if c.priv && gen["PRIV_ARCH"] != "true" {
			t.Errorf("%s/%s: expected PRIV_ARCH=>true, got %v", c.model, c.decode, gen)
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
