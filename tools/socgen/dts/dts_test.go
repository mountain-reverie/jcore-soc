package dts

import (
	"strings"
	"testing"
)

func TestValueFormats(t *testing.T) {
	cases := []struct {
		v    Value
		want string
	}{
		{Str("jcore,uartlite"), `"jcore,uartlite"`},
		{Cells{Nums: []uint64{0x15}, Hex: true}, "<0x15>"},
		{Cells{Nums: []uint64{19200}}, "<19200>"},
		{Cells{Refs: []string{"aic"}}, "<&aic>"},
		{Cells{Refs: []string{"clk"}, Nums: []uint64{0}}, "<&clk 0>"}, // refs before nums
		{Bytes{0x0f, 0x10, 0xff}, "[0f 10 ff]"},                       // zero-pad < 0x10
		{Bytes{}, "[]"},
	}
	for _, c := range cases {
		if got := c.v.render(); got != c.want {
			t.Errorf("render(%#v) = %q, want %q", c.v, got, c.want)
		}
	}
}

// TestBlankSeparator pins the Prop{Name:""} blank-line separator behavior.
func TestBlankSeparator(t *testing.T) {
	n := &Node{Name: "x", Props: []*Prop{
		{Name: "a", Values: []Value{Cells{Nums: []uint64{1}}}},
		{Name: ""}, // blank-line separator
		{Name: "b", Values: []Value{Cells{Nums: []uint64{2}}}},
	}}
	want := "x {\n\ta = <1>;\n\n\tb = <2>;\n};\n"
	if got := renderNode(0, n); got != want {
		t.Errorf("blank separator:\nGOT:  %q\nWANT: %q", got, want)
	}
}

func TestPrintBasics(t *testing.T) {
	root := &Node{Name: "/", Props: []*Prop{
		{Name: "model", Values: []Value{Str("mimas_v2")}},
		{Name: "compatible", Values: []Value{Str("a"), Str("b")}},                 // "a", "b"
		{Name: "interrupt-parent", Values: []Value{Cells{Refs: []string{"aic"}}}}, // <&aic>
		{Name: "#address-cells", Values: []Value{Cells{Nums: []uint64{1}}}},       // <1>
	}, Children: []*Node{
		{Name: "gpio", Props: []*Prop{
			{Name: "compatible", Values: []Value{Str("jcore,gpio1")}},
			{Name: "#gpio-cells", Values: []Value{Cells{Nums: []uint64{2}}}},
			{Name: "gpio-controller"}, // flag
			{Name: "reg", Values: []Value{Cells{Nums: []uint64{0x0, 0x10}, Hex: true}}, Cmt: "ABCD0000-ABCD000F"},
		}},
	}}
	out := Print(root)
	for _, w := range []string{
		"/dts-v1/;",
		`model = "mimas_v2";`,
		`compatible = "a", "b";`,
		"interrupt-parent = <&aic>;",
		"#address-cells = <1>;",
		"#gpio-cells = <2>;",
		"gpio-controller;",
		"reg = <0x0 0x10>; // ABCD0000-ABCD000F",
	} {
		if !strings.Contains(out, w) {
			t.Errorf("Print missing %q:\n%s", w, out)
		}
	}
}

// TestPrintExactFragment pins the EXACT tab/newline formatting against a golden
// fragment copied from targets/boards/mimas_v2/board.dts (the `clocks` node).
// The golden uses TABS. The clocks node has an EMPTY properties list, so the
// only blank line is the automatic separator before the first child node
// (matching the Clojure to-str `interleave (repeat "\n") children`).
func TestPrintExactFragment(t *testing.T) {
	n := &Node{Name: "clocks", Children: []*Node{
		{Name: "bus_clock: bus_clock", Props: []*Prop{
			{Name: "compatible", Values: []Value{Str("fixed-clock")}},
			{Name: "#clock-cells", Values: []Value{Cells{Nums: []uint64{0}}}},
			{Name: "clock-frequency", Values: []Value{Cells{Nums: []uint64{50000000}}}},
		}},
	}}
	got := renderNode(0, n) // unexported; same package test
	// Exact golden text for the `clocks` block at indent 0 (tabs included).
	want := "clocks {\n\n\tbus_clock: bus_clock {\n\t\tcompatible = \"fixed-clock\";\n\t\t#clock-cells = <0>;\n\t\tclock-frequency = <50000000>;\n\t};\n};\n"
	if got != want {
		t.Errorf("renderNode mismatch:\nGOT:\n%q\nWANT:\n%q", got, want)
	}
}

// TestPrintMemoryFragment pins the exact `memory@10000000` block (a node with
// only properties and no children, so no blank line inside it). Golden text
// copied from targets/boards/mimas_v2/board.dts, normalized to indent 0.
func TestPrintMemoryFragment(t *testing.T) {
	n := &Node{Name: "memory@10000000", Props: []*Prop{
		{Name: "device_type", Values: []Value{Str("memory")}},
		{Name: "reg", Values: []Value{Cells{Nums: []uint64{0x10000000, 0x4000000}, Hex: true}}},
	}}
	got := renderNode(0, n)
	want := "memory@10000000 {\n\tdevice_type = \"memory\";\n\treg = <0x10000000 0x4000000>;\n};\n"
	if got != want {
		t.Errorf("renderNode mismatch:\nGOT:\n%q\nWANT:\n%q", got, want)
	}
}

// TestPrintEmptyNode pins the empty-body rendering ("name { };") matching the
// Clojure to-str closing-brace handling (single space when body is empty).
func TestPrintEmptyNode(t *testing.T) {
	got := renderNode(0, &Node{Name: "empty"})
	want := "empty { };\n"
	if got != want {
		t.Errorf("renderNode mismatch:\nGOT:\n%q\nWANT:\n%q", got, want)
	}
}
