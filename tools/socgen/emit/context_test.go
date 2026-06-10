package emit

import (
	"testing"
)

func TestPackagesOf(t *testing.T) {
	lib := libFrom(t, `package data_bus_pack is
  type data_bus_i_t is record a : std_logic; end record;
end package;
package cpu2j0_pack is
  type cpu_data_i_t is record b : std_logic; end record;
end package;`)
	// Marks include duplicates, a std type (skipped), and two work types.
	got := packagesOf(lib, []string{"cpu_data_i_t", "std_logic", "data_bus_i_t", "cpu_data_i_t"})
	want := []string{"cpu2j0_pack", "data_bus_pack"} // distinct + sorted
	if len(got) != len(want) {
		t.Fatalf("packagesOf = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("packagesOf[%d] = %q, want %q (full %v)", i, got[i], want[i], got)
		}
	}
	// nil lib -> empty, no panic.
	if p := packagesOf(nil, []string{"cpu_data_i_t"}); len(p) != 0 {
		t.Errorf("packagesOf(nil) = %v, want empty", p)
	}
}

func TestMergeSortedPkg(t *testing.T) {
	if got := mergeSortedPkg([]string{"cpu2j0_pack", "ddrc_cnt_pack"}, "data_bus_pack"); len(got) != 3 ||
		got[0] != "cpu2j0_pack" || got[1] != "data_bus_pack" || got[2] != "ddrc_cnt_pack" {
		t.Errorf("mergeSortedPkg insert = %v, want [cpu2j0_pack data_bus_pack ddrc_cnt_pack]", got)
	}
	if got := mergeSortedPkg([]string{"data_bus_pack"}, "data_bus_pack"); len(got) != 1 {
		t.Errorf("mergeSortedPkg dup = %v, want [data_bus_pack]", got)
	}
}
