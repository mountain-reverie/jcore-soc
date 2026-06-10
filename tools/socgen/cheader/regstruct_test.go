package cheader

import (
	"strings"
	"testing"

	"github.com/j-core/jcore-soc/tools/socgen/elaborate"
)

func r(name string, lo, hi, width int, mode string) *elaborate.ResolvedReg {
	return &elaborate.ResolvedReg{Name: name, Addr: lo, Width: width, ByteRange: [2]int{lo, hi}, Mode: mode}
}

func TestClassRegStructAligned(t *testing.T) {
	regs := []*elaborate.ResolvedReg{
		r("value", 0, 3, 4, ""), r("mask", 4, 7, 4, ""), r("edge", 8, 11, 4, ""), r("changes", 12, 15, 4, "read"),
	}
	got, ok := classRegStruct("gpio", regs)
	if !ok {
		t.Fatalf("gpio should be aligned:\n%s", got)
	}
	for _, w := range []string{"struct gpio_regs {", "uint32_t value;", "uint32_t changes; // read-only", "};"} {
		if !strings.Contains(got, w) {
			t.Errorf("missing %q:\n%s", w, got)
		}
	}
}

func TestClassRegStructExpandAndIgnore(t *testing.T) {
	regs := []*elaborate.ResolvedReg{r("ctrl", 3, 3, 1, ""), r("data", 7, 7, 1, "")}
	got, ok := classRegStruct("spi", regs)
	if !ok {
		t.Fatalf("spi should align after expansion:\n%s", got)
	}
	for _, w := range []string{"uint32_t ctrl; // only byte 3", "uint32_t data; // only byte 3"} {
		if !strings.Contains(got, w) {
			t.Errorf("missing %q:\n%s", w, got)
		}
	}
}

func TestClassRegStructIgnoreArray(t *testing.T) {
	regs := []*elaborate.ResolvedReg{r("a", 0, 7, 8, ""), r("b", 48, 51, 4, "")}
	got, _ := classRegStruct("x", regs)
	if !strings.Contains(got, "uint32_t a[2];") {
		t.Errorf("8-byte reg should be a[2]:\n%s", got)
	}
	if !strings.Contains(got, "uint32_t ignore0[10];") {
		t.Errorf("40-byte gap should be ignore0[10]:\n%s", got)
	}
}

func TestClassRegStructRightExpand(t *testing.T) {
	// a byte-0 reg with room to the right -> expand right to [0,3] -> "only byte 0".
	regs := []*elaborate.ResolvedReg{r("a", 0, 0, 1, ""), r("b", 4, 7, 4, "")}
	got, ok := classRegStruct("x", regs)
	if !ok {
		t.Fatalf("should align after right-expansion:\n%s", got)
	}
	if !strings.Contains(got, "uint32_t a; // only byte 0") {
		t.Errorf("right-expanded byte-0 reg should be `a; // only byte 0`:\n%s", got)
	}
}

func TestClassRegStructUnaligned(t *testing.T) {
	regs := []*elaborate.ResolvedReg{r("a", 0, 0, 1, ""), r("b", 1, 1, 1, "")}
	if _, ok := classRegStruct("x", regs); ok {
		t.Errorf("wedged sub-word regs should be unaligned (ok=false)")
	}
}
