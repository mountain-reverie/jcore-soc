package iface

import (
	"testing"

	"github.com/j-core/jcore-soc/tools/socgen/vhdl"
)

func TestTypePackage(t *testing.T) {
	df := parse(t, `package cpu2j0_pack is
  type cpu_data_i_t is record
    ack : std_logic;
    data : std_logic_vector(31 downto 0);
  end record;
  subtype byte_t is std_logic_vector(7 downto 0);
end package;`)
	lib, err := Extract([]*vhdl.DesignFile{df})
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if pkg, ok := lib.TypePackage("cpu_data_i_t"); !ok || pkg != "cpu2j0_pack" {
		t.Errorf("TypePackage(cpu_data_i_t) = %q,%v; want cpu2j0_pack,true", pkg, ok)
	}
	if pkg, ok := lib.TypePackage("byte_t"); !ok || pkg != "cpu2j0_pack" {
		t.Errorf("TypePackage(byte_t subtype) = %q,%v; want cpu2j0_pack,true", pkg, ok)
	}
	// A type not declared in any parsed (work) package -> ("", false).
	if pkg, ok := lib.TypePackage("std_logic"); ok {
		t.Errorf("TypePackage(std_logic) = %q,%v; want \"\",false", pkg, ok)
	}
}
