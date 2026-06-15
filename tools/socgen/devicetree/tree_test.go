package devicetree

import (
	"errors"
	"strings"
	"testing"

	"github.com/j-core/jcore-soc/tools/socgen/board"
	"github.com/j-core/jcore-soc/tools/socgen/design"
	"github.com/j-core/jcore-soc/tools/socgen/dts"
	"github.com/j-core/jcore-soc/tools/socgen/elaborate"
	"github.com/j-core/jcore-soc/tools/socgen/iface"
	"github.com/j-core/jcore-soc/tools/socgen/vhdl"
)

// boolp returns a *bool for a literal.
func boolp(b bool) *bool { return &b }

// intp returns a *int for a literal.
func intp(i int) *int { return &i }

// synthBoard builds a synthetic single-cpu board + resolution faithful to the
// mimas single-cpu shape (gpio + aic on cpu0 + uart stdout), enough to exercise
// DeviceTree's boilerplate, soc assembly, and the pit/aic IRQ nodes.
func synthBoard() (*board.Board, *elaborate.Resolution) {
	lib := &iface.Library{Packages: map[string]*iface.Package{
		"config": {Name: "config", Constants: []*iface.Constant{
			{Name: "CFG_CLK_CPU_PERIOD_NS", Value: &vhdl.BasicLit{Kind: vhdl.INT, Value: "20"}},
		}},
	}}

	gpioCls := &design.DeviceClass{
		Entity:      "gpio",
		DtName:      "gpio",
		LeftAddrBit: 3,
		DtProps: design.DtProps{
			{Key: "compatible", Val: "jcore,gpio1"},
			{Key: "gpio-controller", Val: false},
			{Key: "#gpio-cells", Val: []any{2}},
		},
	}
	aicCls := &design.DeviceClass{
		Entity:      "aic",
		DtName:      "interrupt-controller",
		LeftAddrBit: 5,
		DtProps: design.DtProps{
			{Key: "compatible", Val: "jcore,aic1"},
			{Key: "interrupt-controller", Val: false},
			{Key: "#interrupt-cells", Val: []any{1}},
		},
	}
	uartCls := &design.DeviceClass{
		Entity:      "uartlite",
		DtName:      "serial",
		LeftAddrBit: 3,
		DtProps: design.DtProps{
			{Key: "compatible", Val: []any{"jcore,uartlite", "xlnx,xps-uartlite-1.00.a"}},
			{Key: "device_type", Val: "serial"},
		},
	}

	d := &design.Design{
		System: &design.System{},
		DeviceClasses: map[string]*design.DeviceClass{
			"gpio": gpioCls, "aic": aicCls, "uartlite": uartCls,
		},
		Devices: []*design.Device{
			{Class: "gpio", BaseAddr: hexp(0xabcd0000), IRQ: &design.IRQRef{Int: intp(4)}},
			{Class: "aic", Name: "aic0", CPU: intp(0), DtLabel: "aic", BaseAddr: hexp(0xabcd0200)},
			{Class: "uartlite", Name: "uart0", BaseAddr: hexp(0xabcd0100), IRQ: &design.IRQRef{Int: intp(1)},
				DtStdout: true, DtProps: design.DtProps{{Key: "current-speed", Val: []any{19200}}, {Key: "port-number", Val: []any{0}}}},
		},
	}

	res := &elaborate.Resolution{
		Library: lib,
		Classes: map[string]*elaborate.ResolvedClass{
			"gpio": {Name: "gpio", LeftAddrBit: 3},
			"aic":  {Name: "aic", LeftAddrBit: 5},
			"uartlite": {Name: "uartlite", LeftAddrBit: 3},
		},
		Devices: []*elaborate.ResolvedDevice{
			{Name: "gpio", Class: "gpio", BaseAddr: u64p(0xabcd0000), DataBus: true},
			{Name: "aic0", Class: "aic", BaseAddr: u64p(0xabcd0200), DataBus: true,
				Ports: []*elaborate.ResolvedPort{{Name: "irq_i", Kind: elaborate.KindIRQ}}},
			{Name: "uart0", Class: "uartlite", BaseAddr: u64p(0xabcd0100), DataBus: true},
		},
		IRQ: &elaborate.IRQModel{
			VectorNumbers: map[string][8]int{
				"aic0": {0, 0x12 /*uart path1*/, 0, 0, 0x15 /*gpio path4*/, 0, 0, 0},
			},
		},
	}

	return &board.Board{Name: "mimas_v2", Design: d, Library: lib}, res
}

func u64p(v uint64) *uint64 { return &v }

func TestDeviceTreeSingleCPU(t *testing.T) {
	b, res := synthBoard()
	root, err := DeviceTree(b, res)
	if err != nil {
		t.Fatalf("DeviceTree: %v", err)
	}
	out := dts.Print(root)

	for _, w := range []string{
		`model = "mimas_v2";`,
		`compatible = "jcore,j2-soc";`,
		"interrupt-parent = <&aic>;",
		"#address-cells = <1>;",
		"#size-cells = <1>;",
		// chosen / stdout
		`stdout-path = "/soc@abcd0000/serial";`,
		// cpus
		"cpus {",
		"cpu@0 {",
		`device_type = "cpu";`,
		`compatible = "jcore,j2";`,
		"clock-frequency = <50000000>;",
		"reg = <0>;",
		// clocks
		"bus_clock: bus_clock {",
		`compatible = "fixed-clock";`,
		"#clock-cells = <0>;",
		// memory (default dram)
		"memory@10000000 {",
		`device_type = "memory";`,
		"reg = <0x10000000 0x8000000>;",
		// soc
		"soc@abcd0000 {",
		`compatible = "simple-bus";`,
		"ranges = <0x0 0xabcd0000 0x240>;",
		// timer (synthetic pit)
		"timer {",
		`compatible = "jcore,pit";`,
		"reg = <0x200 0x30>;",
		"interrupts = <0x10>;",
		// aic node (label + override reg, no interrupts)
		"aic: interrupt-controller {",
		`compatible = "jcore,aic1";`,
		"interrupt-controller;",
		"#interrupt-cells = <1>;",
		"reg = <0x200 0x40>; // ABCD0200-ABCD023F",
		// gpio device
		"gpio {",
		`compatible = "jcore,gpio1";`,
		"interrupts = <0x15>;",
		// serial device
		"serial {",
		"interrupts = <0x12>;",
		// cpuid
		"cpuid {",
		`compatible = "jcore,cpuid-mmio";`,
		"reg = <0xabcd0600 0x4>;",
	} {
		if !strings.Contains(out, w) {
			t.Errorf("DeviceTree output missing %q:\n%s", w, out)
		}
	}

	// soc child order: timer < aic < gpio < serial (timer & aic first, then
	// data-bus devices sorted by node-name; aic sorts first via its label).
	iTimer := strings.Index(out, "timer {")
	iAic := strings.Index(out, "aic: interrupt-controller {")
	iGpio := strings.Index(out, "gpio {")
	iSerial := strings.Index(out, "serial {")
	if !(iTimer < iAic && iAic < iGpio && iGpio < iSerial) {
		t.Errorf("soc child order wrong (want timer<aic<gpio<serial):\n%s", out)
	}

	// the aic must NOT also appear as a plain device node, and must carry no
	// interrupts property.
	aicNode := out[iAic:iGpio]
	if strings.Contains(aicNode, "interrupts") {
		t.Errorf("aic controller node must not have interrupts:\n%s", aicNode)
	}

	// single-cpu: no SMP additions.
	for _, w := range []string{"enable-method", "cpu@1", "jcore,ipi-controller"} {
		if strings.Contains(out, w) {
			t.Errorf("single-cpu board must not emit %q:\n%s", w, out)
		}
	}
}

// smpBoard builds a synthetic 2-cpu board: cpu1 peripheral bus present, a second
// aic (aic1 on cpu1), and a cache_ctrl device carrying the ipi base-addr + irq.
func smpBoard() (*board.Board, *elaborate.Resolution) {
	b, res := synthBoard()
	b.Design.PeripheralBuses = map[string]bool{"cpu1": true}

	cacheCls := &design.DeviceClass{Entity: "cache_ctrl", DtName: "cache", LeftAddrBit: 3}
	b.Design.DeviceClasses["cache_ctrl"] = cacheCls

	// second aic on cpu1.
	b.Design.Devices = append(b.Design.Devices,
		&design.Device{Class: "aic", Name: "aic1", CPU: intp(1), DtLabel: "aic1", BaseAddr: hexp(0xabcd0300)},
		&design.Device{Class: "cache_ctrl", Name: "cache0", BaseAddr: hexp(0xabcd0400),
			IRQ: &design.IRQRef{Named: map[string]*design.IRQEntry{"int0": {CPU: 0, IRQ: 3}}}},
	)
	res.Classes["cache_ctrl"] = &elaborate.ResolvedClass{Name: "cache_ctrl", LeftAddrBit: 3}
	res.Devices = append(res.Devices,
		&elaborate.ResolvedDevice{Name: "aic1", Class: "aic", BaseAddr: u64p(0xabcd0300), DataBus: true},
		&elaborate.ResolvedDevice{Name: "cache0", Class: "cache_ctrl", BaseAddr: u64p(0xabcd0400), DataBus: true},
	)
	return b, res
}

func TestDeviceTreeSMP(t *testing.T) {
	b, res := smpBoard()
	root, err := DeviceTree(b, res)
	if err != nil {
		t.Fatalf("DeviceTree (SMP): %v", err)
	}
	out := dts.Print(root)

	for _, w := range []string{
		`enable-method = "jcore,spin-table";`,
		"cpu@1 {",
		"reg = <1>;",
		"cpu-release-addr = <0xabcd0640 0x8000>;",
		// ipi node
		"ipi {",
		`compatible = "jcore,ipi-controller";`,
		"reg = <0xabcd0400 0x8>;",
		"interrupts = <0x14>;", // AIC vector = 0x11 + raw irq 3
	} {
		if !strings.Contains(out, w) {
			t.Errorf("SMP DeviceTree output missing %q:\n%s", w, out)
		}
	}

	// cpu@0 and cpu@1 both present, cpu@0 reg<0>, cpu@1 reg<1>.
	if !strings.Contains(out, "cpu@0 {") {
		t.Errorf("SMP must still emit cpu@0:\n%s", out)
	}
}

// TestDeviceTreeSMPMultiIPIIRQ locks the >1 distinct ipi irq validation error.
func TestDeviceTreeSMPMultiIPIIRQ(t *testing.T) {
	b, res := smpBoard()
	// give the cache two distinct irqs.
	for _, d := range b.Design.Devices {
		if d.Class == "cache_ctrl" {
			d.IRQ = &design.IRQRef{Named: map[string]*design.IRQEntry{
				"int0": {CPU: 0, IRQ: 3},
				"int1": {CPU: 1, IRQ: 5},
			}}
		}
	}
	_, err := DeviceTree(b, res)
	if err == nil {
		t.Fatalf("expected DTError for multiple ipi irqs, got nil")
	}
	var de *DTError
	if !errors.As(err, &de) || !errors.Is(err, ErrIPI) {
		t.Errorf("want DTError{ErrIPI}, got %v", err)
	}
}

func TestDeviceTreeNilSystemDefaultDram(t *testing.T) {
	b, res := synthBoard()
	b.Design.System = nil
	root, err := DeviceTree(b, res)
	if err != nil {
		t.Fatalf("DeviceTree (nil System): %v", err)
	}
	out := dts.Print(root)
	if !strings.Contains(out, "memory@10000000 {") || !strings.Contains(out, "reg = <0x10000000 0x8000000>;") {
		t.Errorf("nil System should use default dram:\n%s", out)
	}
}

// TestDeviceTreeNoAic locks the Fix-1 guard: a board with no aic device emits
// neither the synthetic timer (jcore,pit) nor an interrupt-controller node, and
// must not error even though pitTrap is never consulted.
func TestDeviceTreeNoAic(t *testing.T) {
	b, res := synthBoard()
	// drop the aic device (index 1) from both the design and the resolution,
	// keeping the two collections index-aligned (the P5e invariant).
	b.Design.Devices = []*design.Device{b.Design.Devices[0], b.Design.Devices[2]}
	res.Devices = []*elaborate.ResolvedDevice{res.Devices[0], res.Devices[2]}
	delete(b.Design.DeviceClasses, "aic")
	delete(res.Classes, "aic")

	root, err := DeviceTree(b, res)
	if err != nil {
		t.Fatalf("DeviceTree (no aic): %v", err)
	}
	out := dts.Print(root)
	if strings.Contains(out, "jcore,pit") {
		t.Errorf("no-aic board must not emit a timer (jcore,pit) node:\n%s", out)
	}
	if strings.Contains(out, "interrupt-controller") {
		t.Errorf("no-aic board must not emit an interrupt-controller node:\n%s", out)
	}
}

func TestDeviceTreeDtNodeFalseExcluded(t *testing.T) {
	b, res := synthBoard()
	// suppress the gpio dt node.
	b.Design.Devices[0].DtNode = boolp(false)
	root, err := DeviceTree(b, res)
	if err != nil {
		t.Fatalf("DeviceTree: %v", err)
	}
	out := dts.Print(root)
	if strings.Contains(out, "jcore,gpio1") {
		t.Errorf("gpio with dt-node:false must be excluded:\n%s", out)
	}
}
