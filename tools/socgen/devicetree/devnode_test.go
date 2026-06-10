package devicetree

import (
	"strings"
	"testing"

	"github.com/j-core/jcore-soc/tools/socgen/design"
	"github.com/j-core/jcore-soc/tools/socgen/dts"
)

// hexp builds a *design.Hex from a uint64 (design.Device.BaseAddr is *Hex).
func hexp(v uint64) *design.Hex {
	h := design.Hex(v)
	return &h
}

// printNode renders a single node by wrapping it under a throwaway root so the
// dts printer (which takes a root) can be reused in assertions.
func printNode(n *dts.Node) string {
	return dts.Print(&dts.Node{Name: "root", Children: []*dts.Node{n}})
}

func TestDevToDTGpio(t *testing.T) {
	dev := &design.Device{Class: "gpio", Name: "gpio", BaseAddr: hexp(0xabcd0000)}
	cls := &design.DeviceClass{
		DtProps: map[string]any{
			"compatible":      "jcore,gpio1",
			"gpio-controller": false,
			"#gpio-cells":     []any{2},
		},
		LeftAddrBit: 3,
	}
	n := devToDT(dev, cls, "gpio", 0xabcd0000, 0x15, true)
	out := printNode(n)
	for _, w := range []string{
		`compatible = "jcore,gpio1";`,
		"gpio-controller;",
		"#gpio-cells = <2>;",
		"reg = <0x0 0x10>; // ABCD0000-ABCD000F",
		"interrupts = <0x15>;",
	} {
		if !strings.Contains(out, w) {
			t.Errorf("devToDT gpio missing %q:\n%s", w, out)
		}
	}
	// dt-props are sorted: compatible before gpio-controller before #gpio-cells?
	// Sorted ASCII: "#gpio-cells" < "compatible" < "gpio-controller".
	iHash := strings.Index(out, "#gpio-cells")
	iCompat := strings.Index(out, "compatible")
	iGpio := strings.Index(out, "gpio-controller")
	if !(iHash < iCompat && iCompat < iGpio) {
		t.Errorf("dt-props not sorted (#gpio-cells<compatible<gpio-controller):\n%s", out)
	}
}

func TestDevToDTMultiStringCompatible(t *testing.T) {
	dev := &design.Device{Class: "serial", Name: "serial", BaseAddr: hexp(0xabcd0100)}
	cls := &design.DeviceClass{
		DtProps: map[string]any{
			"compatible": []any{"jcore,uartlite", "xlnx,xps-uartlite-1.00.a"},
		},
		LeftAddrBit: 3,
	}
	n := devToDT(dev, cls, "serial", 0xabcd0000, 0x12, true)
	out := printNode(n)
	want := `compatible = "jcore,uartlite", "xlnx,xps-uartlite-1.00.a";`
	if !strings.Contains(out, want) {
		t.Errorf("multi-string compatible missing %q:\n%s", want, out)
	}
	// reg relative to busBase 0xabcd0000 -> 0x100; width 1<<4 = 0x10.
	if !strings.Contains(out, "reg = <0x100 0x10>; // ABCD0100-ABCD010F") {
		t.Errorf("relative reg wrong:\n%s", out)
	}
	// hasIRQ true and non-cache device -> interrupts MUST be present (guards
	// against a broadened irq-suppression bug).
	if !strings.Contains(out, "interrupts = <0x12>;") {
		t.Errorf("interrupts missing despite hasIRQ=true non-cache device:\n%s", out)
	}
}

func TestDevToDTCellsAndPhandle(t *testing.T) {
	dev := &design.Device{Class: "spi", Name: "spi", BaseAddr: hexp(0xabcd0040)}
	cls := &design.DeviceClass{
		DtProps: map[string]any{
			"voltage-ranges":    []any{3200, 3400},
			"spi-max-frequency": []any{25000000},
			"clocks":            []any{"&bus_clock"},
			"clock-names":       "ref_clk",
			"status":            "ok",
		},
		LeftAddrBit: 2,
	}
	n := devToDT(dev, cls, "spi", 0xabcd0000, 0, false)
	out := printNode(n)
	for _, w := range []string{
		"voltage-ranges = <3200 3400>;",
		"spi-max-frequency = <25000000>;",
		"clocks = <&bus_clock>;",
		`clock-names = "ref_clk";`,
		`status = "ok";`,
	} {
		if !strings.Contains(out, w) {
			t.Errorf("spi props missing %q:\n%s", w, out)
		}
	}
	// hasIRQ false -> no interrupts.
	if strings.Contains(out, "interrupts") {
		t.Errorf("interrupts present despite hasIRQ=false:\n%s", out)
	}
}

func TestDevToDTCacheNoInterrupts(t *testing.T) {
	dev := &design.Device{Class: "cache", Name: "cache-controller", BaseAddr: hexp(0xabcd00c0)}
	cls := &design.DeviceClass{
		DtProps: map[string]any{
			"compatible": "jcore,cache",
			"cpu-offset": []any{4},
		},
		LeftAddrBit: 5,
	}
	// hasIRQ true, but compatible==jcore,cache must suppress interrupts.
	n := devToDT(dev, cls, "cache-controller", 0xabcd0000, 0x9, true)
	out := printNode(n)
	if strings.Contains(out, "interrupts") {
		t.Errorf("cache device must NOT emit interrupts:\n%s", out)
	}
	if !strings.Contains(out, "cpu-offset = <4>;") {
		t.Errorf("cpu-offset missing:\n%s", out)
	}
}

func TestDevToDTDeviceOverridesClass(t *testing.T) {
	dev := &design.Device{
		Class: "gpio", Name: "gpio", BaseAddr: hexp(0xabcd0000),
		DtProps: map[string]any{"compatible": "jcore,gpio-override"},
	}
	cls := &design.DeviceClass{
		DtProps:     map[string]any{"compatible": "jcore,gpio1"},
		LeftAddrBit: 3,
	}
	n := devToDT(dev, cls, "gpio", 0xabcd0000, 0, false)
	out := printNode(n)
	if !strings.Contains(out, `compatible = "jcore,gpio-override";`) {
		t.Errorf("device dt-props should override class:\n%s", out)
	}
}

func TestDevToDTExplicitRegRelativized(t *testing.T) {
	// An explicit dt-props reg is relativized to busBase, and the default reg
	// is NOT added.
	dev := &design.Device{Class: "x", Name: "x", BaseAddr: hexp(0xabcd0200)}
	cls := &design.DeviceClass{
		DtProps:     map[string]any{"reg": []any{0xabcd0200, 0x40}},
		LeftAddrBit: 5,
	}
	n := devToDT(dev, cls, "x", 0xabcd0000, 0, false)
	out := printNode(n)
	// Only one reg line; relativized first cell (0x200), second cell unchanged.
	if strings.Count(out, "reg =") != 1 {
		t.Errorf("expected exactly one reg line:\n%s", out)
	}
	if !strings.Contains(out, "reg = <512 64>;") {
		t.Errorf("explicit reg relativized to decimal cells expected <512 64>:\n%s", out)
	}
}

func TestToUint64(t *testing.T) {
	// yaml.v3 decodes scientific-notation integers (e.g. 25e6) as float64.
	if got, ok := toUint64(float64(25e6)); !ok || got != 25000000 {
		t.Errorf("toUint64(float64(25e6)) = (%d, %v); want (25000000, true)", got, ok)
	}
	// A negative signed int must be rejected, not silently wrapped.
	if got, ok := toUint64(int(-1)); ok {
		t.Errorf("toUint64(int(-1)) = (%d, %v); want (_, false)", got, ok)
	}
	// A plain positive int literal still works.
	if got, ok := toUint64(int(42)); !ok || got != 42 {
		t.Errorf("toUint64(int(42)) = (%d, %v); want (42, true)", got, ok)
	}
}

func TestDevToDTChildren(t *testing.T) {
	dev := &design.Device{Class: "spi", Name: "spi", BaseAddr: hexp(0xabcd0040)}
	cls := &design.DeviceClass{
		DtProps:     map[string]any{"compatible": "jcore,spi2"},
		LeftAddrBit: 2,
		DtChildren: []any{
			[]any{
				"sdcard@0",
				map[string]any{
					"properties": map[string]any{
						"compatible":     "mmc-spi-slot",
						"reg":            []any{0},
						"voltage-ranges": []any{3200, 3400},
						"m25p,fast-read": false,
					},
				},
			},
			[]any{
				"m25p80@1",
				map[string]any{
					"properties": map[string]any{
						"compatible": []any{"s25fl164k", "jedec,spi-nor"},
					},
					"children": []any{
						[]any{
							"partition@0",
							map[string]any{
								"properties": map[string]any{
									"label": "spi_flash",
									"reg":   []any{0, 0},
								},
							},
						},
					},
				},
			},
		},
	}
	n := devToDT(dev, cls, "spi", 0xabcd0000, 0, false)
	out := printNode(n)
	for _, w := range []string{
		"sdcard@0 {",
		`compatible = "mmc-spi-slot";`,
		"reg = <0>;",
		"voltage-ranges = <3200 3400>;",
		"m25p,fast-read;",
		"m25p80@1 {",
		`compatible = "s25fl164k", "jedec,spi-nor";`,
		"partition@0 {",
		`label = "spi_flash";`,
		"reg = <0 0>;",
	} {
		if !strings.Contains(out, w) {
			t.Errorf("dt-children missing %q:\n%s", w, out)
		}
	}
}
