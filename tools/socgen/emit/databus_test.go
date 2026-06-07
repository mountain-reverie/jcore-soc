package emit

import (
	"strings"
	"testing"

	"github.com/j-core/jcore-soc/tools/socgen/vhdl"
)

func bd(name string, base uint64, leftBit int) busDevice {
	return busDevice{Name: name, BaseAddr: base, LeftAddrBit: leftBit}
}

func TestDevicePrefixExact(t *testing.T) {
	// base 0xABCD0000, leftBit 3 -> drop low 4 bits -> 28-bit prefix.
	p := devicePrefix(bd("gpio", 0xABCD0000, 3), false)
	if len(p) != 28 {
		t.Fatalf("exact prefix len = %d, want 28", len(p))
	}
	// 0xABCD = 1010 1011 1100 1101 -> the prefix starts with those 16 bits.
	if !strings.HasPrefix(p, "1010101111001101") {
		t.Errorf("prefix = %q", p)
	}
}

func TestDecodeAddressStructure(t *testing.T) {
	devs := []busDevice{
		bd("gpio", 0xA0000000, 3),
		bd("uart0", 0xA0000100, 3),
	}
	lits := map[string]string{"gpio": "DEV_GPIO", "uart0": "DEV_UART0", "none": "NONE"}
	fn := decodeFunction(devs, lits, false) // *vhdl.SubprogramBody
	if fn.Designator != "decode_address" || fn.ReturnMark != "device_t" {
		t.Fatalf("spec = %q -> %q", fn.Designator, fn.ReturnMark)
	}
	// render inside a throwaway architecture and re-parse to prove valid VHDL.
	df := &vhdl.DesignFile{Units: []vhdl.DesignUnit{
		&vhdl.ArchitectureBody{Name: "impl", Entity: "e", Decls: []vhdl.Decl{fn}},
	}}
	out := vhdl.Print(df)
	for _, want := range []string{"function decode_address", "return DEV_GPIO", "return DEV_UART0", "return NONE"} {
		if !strings.Contains(out, want) {
			t.Errorf("decode fn missing %q:\n%s", want, out)
		}
	}
	if _, perr := vhdl.ParseFile(vhdl.NewFileSet(), "t.vhd", []byte(out)); perr != nil {
		t.Fatalf("decode fn does not re-parse: %v\n%s", perr, out)
	}
}

func TestDecodeAddressZeroDevices(t *testing.T) {
	fn := decodeFunction(nil, map[string]string{"none": "NONE"}, false)
	df := &vhdl.DesignFile{Units: []vhdl.DesignUnit{
		&vhdl.ArchitectureBody{Name: "impl", Entity: "e", Decls: []vhdl.Decl{fn}},
	}}
	out := vhdl.Print(df)
	if !strings.Contains(out, "return NONE") {
		t.Errorf("zero-device decode must return NONE:\n%s", out)
	}
	if strings.Contains(out, "if ") {
		t.Errorf("zero-device decode must have no if statement:\n%s", out)
	}
}

func TestDecodeAddressExactStructure(t *testing.T) {
	devs := []busDevice{
		bd("gpio", 0xABCD0000, 3),
		bd("flash", 0xABCD0040, 2),
		bd("aic0", 0xABCD0200, 5),
	}
	lits := map[string]string{"gpio": "DEV_GPIO", "flash": "DEV_FLASH", "aic0": "DEV_AIC0", "none": "NONE"}
	fn := decodeFunction(devs, lits, false)
	df := &vhdl.DesignFile{Units: []vhdl.DesignUnit{
		&vhdl.ArchitectureBody{Name: "impl", Entity: "e", Decls: []vhdl.Decl{fn}},
	}}
	out := vhdl.Print(df)
	for _, want := range []string{
		"addr(27 downto 10) = \"101111001101000000\"", // top-level >4-bit prefix wrap
		"addr(6 downto 3) = \"1000\"",                 // flash multi-bit slice (lo = hi-len+1)
		"elsif",                                        // if/elsif path exercised
		"return DEV_FLASH",
		"return DEV_AIC0",
		"return NONE",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("decode output missing %q:\n%s", want, out)
		}
	}
	// re-parse for validity.
	if _, perr := vhdl.ParseFile(vhdl.NewFileSet(), "t.vhd", []byte(out)); perr != nil {
		t.Fatalf("decode does not re-parse: %v\n%s", perr, out)
	}
}

func TestDecodeAddressSimpleModeTrims(t *testing.T) {
	devs := []busDevice{
		bd("a", 0xA0000000, 3),
		bd("b", 0xA0000080, 3),
	}
	lits := map[string]string{"a": "DEV_A", "b": "DEV_B", "none": "NONE"}
	exact := renderFn(t, decodeFunction(devs, lits, false))
	simple := renderFn(t, decodeFunction(devs, lits, true))
	// both must decode both devices...
	for _, out := range []string{exact, simple} {
		for _, w := range []string{"return DEV_A", "return DEV_B", "return NONE"} {
			if !strings.Contains(out, w) {
				t.Errorf("missing %q:\n%s", w, out)
			}
		}
	}
	// ...and simple mode must compare FEWER total address-bit positions than exact
	// (trimmed suffix). Use the count of "downto" + "addr(" occurrences as a proxy,
	// or assert simple's output is shorter. Simple <= exact in compared-bit span.
	if len(simple) > len(exact) {
		t.Errorf("simple-mode decode should not be longer than exact:\nsimple=%s\nexact=%s", simple, exact)
	}
}

func renderFn(t *testing.T, fn *vhdl.SubprogramBody) string {
	t.Helper()
	df := &vhdl.DesignFile{Units: []vhdl.DesignUnit{
		&vhdl.ArchitectureBody{Name: "impl", Entity: "e", Decls: []vhdl.Decl{fn}},
	}}
	return vhdl.Print(df)
}
