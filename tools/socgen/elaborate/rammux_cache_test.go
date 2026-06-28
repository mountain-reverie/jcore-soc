package elaborate

import "testing"

func TestRAMMuxConfigNameMapping(t *testing.T) {
	cfg, _, err := RAMMuxConfig(1, "id")
	if err != nil || cfg != "ddr_ram_mux_one_cpu_idcache_fpga" {
		t.Fatalf("got %q err %v", cfg, err)
	}
}
