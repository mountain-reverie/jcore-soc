package iface

import (
	"testing"

	"github.com/j-core/jcore-soc/tools/socgen/vhdl"
)

func TestIsConstant(t *testing.T) {
	df := parse(t, `package p is
  constant K : integer := 1;
  type t_t is (a, b);
end package;`)
	lib, err := Extract([]*vhdl.DesignFile{df})
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if !lib.IsConstant("K") || !lib.IsConstant("k") {
		t.Errorf("IsConstant(K/k) should be true")
	}
	if lib.IsConstant("t_t") {
		t.Errorf("IsConstant(t_t) should be false (it's a type)")
	}
	if lib.IsConstant("nope") {
		t.Errorf("IsConstant(nope) should be false")
	}
}
