package elaborate

import (
	"testing"

	"github.com/j-core/jcore-soc/tools/socgen/design"
)

func TestResolvePio(t *testing.T) {
	c0 := 0
	d := &design.Design{System: &design.System{Pio: design.PioMap{
		{Lo: 3, Hi: 4, Const: &c0}, // const bits 3,4 (listed first: exercises the sort)
		{Lo: 0, Hi: 2, Name: "x"},  // loopback bits 0,1,2
	}}}
	bits := resolvePio(d)
	if len(bits) != 5 {
		t.Fatalf("want 5 bits, got %d: %+v", len(bits), bits)
	}
	for i, b := range bits {
		if b.Idx != i {
			t.Errorf("bit %d has Idx %d", i, b.Idx)
		}
		if i < 3 && b.Const != nil {
			t.Errorf("bit %d should be loopback (Const nil), got %v", i, *b.Const)
		}
		if i >= 3 && (b.Const == nil || *b.Const != 0) {
			t.Errorf("bit %d should be const 0, got %v", i, b.Const)
		}
	}
}

func TestResolvePioNil(t *testing.T) {
	if bits := resolvePio(&design.Design{}); len(bits) != 0 {
		t.Errorf("nil System -> want 0 bits, got %d", len(bits))
	}
}
