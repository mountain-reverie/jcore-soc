package devicetree

import (
	"fmt"
	"sort"
	"strings"

	"github.com/j-core/jcore-soc/tools/socgen/board"
	"github.com/j-core/jcore-soc/tools/socgen/design"
	"github.com/j-core/jcore-soc/tools/socgen/dts"
	"github.com/j-core/jcore-soc/tools/socgen/elaborate"
)

// aicClass is the device-class name of the AIC1 interrupt controller.
const aicClass = "aic"

// countdownTrap is aic.vhd's internal countdown-timer trap, always reserved
// when picking the pit trap (irq.clj).
const countdownTrap = 0x19

// vectorBase is the first AIC vector number; a device irq line n maps to
// 0x11+n (irq.clj, "vector numbers start at 0x11 based on v_irq_t of aic.vhd").
const vectorBase = 0x11

// dtDevice is a selected (data-bus, dt-node) device paired with its resolved
// info, effective dt-name/label, and computed geometry.
type dtDevice struct {
	dev      *design.Device
	cls      *design.DeviceClass
	name     string // effective instance name (res.Devices[i].Name)
	base     uint64 // absolute base address
	dtName   string // dt node base name (class default)
	dtLabel  string // dt label ("" if none)
	regWidth uint64
}

// nodeName builds the Clojure node-name "label: name" prefix used both for the
// rendered node and (critically) for the soc-child sort key.
func nodeName(label, name string) string {
	if label != "" {
		return label + ": " + name
	}
	return name
}

// DeviceTree assembles the single-cpu device tree for a board, faithful to
// device_tree.clj to-dt plus the aic/pit IRQ nodes from irq.clj. It returns the
// root "/" node. SMP/ipi (Task 5) and the BoardDTS entry/golden parity (Task 6)
// are layered on top of this.
func DeviceTree(b *board.Board, res *elaborate.Resolution) (*dts.Node, error) {
	freq, err := cpuFreq(b.Library)
	if err != nil {
		return nil, err
	}

	dram := [2]uint64{0x10000000, 0x8000000}
	if b.Design.System != nil {
		dram = b.Design.System.DramOr()
	}

	sel := selectDevices(b, res)

	// bus base = min selected base; bus width = max(base+regWidth) - busBase.
	busBase := uint64(0)
	if len(sel) > 0 {
		busBase = sel[0].base
		for _, s := range sel {
			if s.base < busBase {
				busBase = s.base
			}
		}
	}
	var busWidth uint64
	for _, s := range sel {
		if end := s.base + s.regWidth - busBase; end > busWidth {
			busWidth = end
		}
	}

	// dt-name reuse -> @addr suffix.
	nameFreq := map[string]int{}
	for _, s := range sel {
		nameFreq[s.dtName]++
	}

	// build soc children. The synthetic timer is a soc dt-child prepended
	// unsorted (irq.clj conjs it onto :soc :dt-children, which to-dt concats
	// BEFORE the sorted device nodes). Only the device nodes — including the
	// reg-overridden aic — are sorted by node-name.
	type child struct {
		key  string
		node *dts.Node
	}
	children := make([]child, 0, len(sel))

	// aic base offset (for the synthetic timer reg + aic reg override).
	var aicBase uint64
	var haveAic bool
	for _, s := range sel {
		if lc(s.dev.Class) == aicClass {
			aicBase = s.base
			haveAic = true
			break
		}
	}

	// pit trap = first free trap in 16..31 of {countdown} ∪ all device vectors.
	// Only computed when an aic is present, since the synthetic timer (and thus
	// the pit trap) is only emitted under haveAic; a board with no aic must not
	// fail on an exhausted trap space it would never use.
	var trap int
	if haveAic {
		var err error
		trap, err = pitTrap(b)
		if err != nil {
			return nil, err
		}
	}

	socChildren := make([]*dts.Node, 0, len(sel)+1)
	if haveAic {
		aicOff := aicBase - busBase
		timer := &dts.Node{Name: "timer", Props: []*dts.Prop{
			{Name: "compatible", Values: []dts.Value{dts.Str("jcore,pit")}},
			{Name: "reg", Values: []dts.Value{dts.Cells{Nums: []uint64{aicOff, 0x30}, Hex: true}}},
			{Name: "interrupts", Values: []dts.Value{dts.Cells{Nums: []uint64{uint64(trap)}, Hex: true}}},
		}}
		socChildren = append(socChildren, timer)
	}

	for _, s := range sel {
		name := s.dtName
		if nameFreq[s.dtName] > 1 {
			name = fmt.Sprintf("%s@%x", s.dtName, s.base-busBase)
		}
		nn := nodeName(s.dtLabel, name)

		if lc(s.dev.Class) == aicClass {
			// the aic is built as a device node but with its reg overridden to
			// the combined aic+pit region <aicOff regWidth>, and it carries no
			// interrupts (it is the controller).
			node := devToDT(s.dev, s.cls, nn, busBase, 0, false)
			aicOff := s.base - busBase
			overrideReg(node, aicOff, s.regWidth, fmt.Sprintf("%08X-%08X", s.base, s.base+s.regWidth-1))
			children = append(children, child{key: nn, node: node})
			continue
		}

		vec, hasIRQ := deviceVector(s.dev)
		node := devToDT(s.dev, s.cls, nn, busBase, vec, hasIRQ)
		children = append(children, child{key: nn, node: node})
	}

	sort.SliceStable(children, func(i, j int) bool { return children[i].key < children[j].key })
	for _, c := range children {
		socChildren = append(socChildren, c.node)
	}

	// stdout device -> "/soc@<busBaseHex>/<node-name>".
	stdoutPath := ""
	for _, s := range sel {
		if s.dev.DtStdout {
			name := s.dtName
			if nameFreq[s.dtName] > 1 {
				name = fmt.Sprintf("%s@%x", s.dtName, s.base-busBase)
			}
			stdoutPath = fmt.Sprintf("/soc@%x/%s", busBase, name)
			break
		}
	}

	return buildRoot(b.Name, freq, dram, busBase, busWidth, stdoutPath, socChildren), nil
}

// buildRoot assembles the "/" boilerplate around the soc children.
func buildRoot(model string, freq int, dram [2]uint64, busBase, busWidth uint64, stdoutPath string, socChildren []*dts.Node) *dts.Node {
	cpuProps := []*dts.Prop{
		{Name: "device_type", Values: []dts.Value{dts.Str("cpu")}},
		{Name: "compatible", Values: []dts.Value{dts.Str("jcore,j2")}},
		{Name: "clock-frequency", Values: []dts.Value{dts.Cells{Nums: []uint64{uint64(freq)}}}},
		{Name: "reg", Values: []dts.Value{dts.Cells{Nums: []uint64{0}}}},
	}

	chosen := &dts.Node{Name: "chosen"}
	if stdoutPath != "" {
		chosen.Props = []*dts.Prop{{Name: "stdout-path", Values: []dts.Value{dts.Str(stdoutPath)}}}
	}

	cpus := &dts.Node{Name: "cpus", Props: []*dts.Prop{
		{Name: "#address-cells", Values: []dts.Value{dts.Cells{Nums: []uint64{1}}}},
		{Name: "#size-cells", Values: []dts.Value{dts.Cells{Nums: []uint64{0}}}},
	}, Children: []*dts.Node{
		{Name: "cpu@0", Props: cpuProps},
	}}

	clocks := &dts.Node{Name: "clocks", Children: []*dts.Node{
		{Name: nodeName("bus_clock", "bus_clock"), Props: []*dts.Prop{
			{Name: "compatible", Values: []dts.Value{dts.Str("fixed-clock")}},
			{Name: "#clock-cells", Values: []dts.Value{dts.Cells{Nums: []uint64{0}}}},
			{Name: "clock-frequency", Values: []dts.Value{dts.Cells{Nums: []uint64{uint64(freq)}}}},
		}},
	}}

	memory := &dts.Node{Name: fmt.Sprintf("memory@%x", dram[0]), Props: []*dts.Prop{
		{Name: "device_type", Values: []dts.Value{dts.Str("memory")}},
		{Name: "reg", Values: []dts.Value{dts.Cells{Nums: []uint64{dram[0], dram[1]}, Hex: true}}},
	}}

	soc := &dts.Node{Name: fmt.Sprintf("soc@%x", busBase), Props: []*dts.Prop{
		{Name: "compatible", Values: []dts.Value{dts.Str("simple-bus")}},
		{Name: "ranges", Values: []dts.Value{dts.Cells{Nums: []uint64{0, busBase, busWidth}, Hex: true}}},
		{Name: "#address-cells", Values: []dts.Value{dts.Cells{Nums: []uint64{1}}}},
		{Name: "#size-cells", Values: []dts.Value{dts.Cells{Nums: []uint64{1}}}},
	}, Children: socChildren}

	cpuid := &dts.Node{Name: "cpuid", Props: []*dts.Prop{
		{Name: "compatible", Values: []dts.Value{dts.Str("jcore,cpuid-mmio")}},
		{Name: "reg", Values: []dts.Value{dts.Cells{Nums: []uint64{0xabcd0600, 0x4}, Hex: true}}},
	}}

	return &dts.Node{Name: "/", Props: []*dts.Prop{
		{Name: "model", Values: []dts.Value{dts.Str(model)}},
		{Name: "compatible", Values: []dts.Value{dts.Str("jcore,j2-soc")}},
		{Name: "#address-cells", Values: []dts.Value{dts.Cells{Nums: []uint64{1}}}},
		{Name: "#size-cells", Values: []dts.Value{dts.Cells{Nums: []uint64{1}}}},
		// interrupt-parent points at the aic by phandle. This hardcodes aicClass
		// ("aic") as the label, assuming the J-core board invariant that the aic
		// device's dt-label is "aic" (matching aicClass). We do not derive it
		// from the device's DtLabel.
		{Name: "interrupt-parent", Values: []dts.Value{dts.Cells{Refs: []string{aicClass}}}},
	}, Children: []*dts.Node{chosen, cpus, clocks, memory, soc, cpuid}}
}

// selectDevices filters b.Design.Devices to data-bus devices that want a dt
// node, index-aligned with res.Devices (the P5e invariant), and computes each
// one's effective name/dt-name/label/geometry.
func selectDevices(b *board.Board, res *elaborate.Resolution) []dtDevice {
	out := make([]dtDevice, 0, len(b.Design.Devices))
	for i, dev := range b.Design.Devices {
		if res == nil || i >= len(res.Devices) {
			break
		}
		rd := res.Devices[i]
		if !rd.DataBus {
			continue
		}
		if dev.DtNode != nil && !*dev.DtNode {
			continue
		}
		cls := b.Design.DeviceClasses[dev.Class]
		if cls == nil {
			continue
		}
		base := uint64(0)
		if dev.BaseAddr != nil {
			base = uint64(*dev.BaseAddr)
		}
		dtName := dev.Class
		if cls.DtName != "" {
			dtName = cls.DtName
		}
		out = append(out, dtDevice{
			dev:      dev,
			cls:      cls,
			name:     rd.Name,
			base:     base,
			dtName:   dtName,
			dtLabel:  dev.DtLabel,
			regWidth: uint64(1) << (cls.LeftAddrBit + 1),
		})
	}
	return out
}

// deviceVector returns the dt interrupt vector for a device and whether an
// interrupts property should be emitted, faithful to
// (some #(if (:dt? %) %) (vals (:irq dev))) with vector 0x11+irq. A bare
// `irq: n` is always dt; a named irq set selects the first (sorted) entry whose
// dt? is not false.
func deviceVector(dev *design.Device) (int, bool) {
	if dev.IRQ == nil {
		return 0, false
	}
	if dev.IRQ.Int != nil {
		return vectorBase + *dev.IRQ.Int, true
	}
	for _, k := range sortedNamedKeys(dev.IRQ.Named) {
		e := dev.IRQ.Named[k]
		if e.DT == nil || *e.DT {
			return vectorBase + e.IRQ, true
		}
	}
	return 0, false
}

// pitTrap selects the first unused trap in 16..31 of {countdownTrap} ∪ all
// device vector numbers (0x11+irq), including dt?:false irqs (irq.clj all-irq).
func pitTrap(b *board.Board) (int, error) {
	used := map[int]bool{countdownTrap: true}
	for _, dev := range b.Design.Devices {
		if dev.IRQ == nil {
			continue
		}
		switch {
		case dev.IRQ.Int != nil:
			used[vectorBase+*dev.IRQ.Int] = true
		case dev.IRQ.Named != nil:
			for _, e := range dev.IRQ.Named {
				used[vectorBase+e.IRQ] = true
			}
		}
	}
	for t := 16; t < 32; t++ {
		if !used[t] {
			return t, nil
		}
	}
	return 0, &DTError{Kind: ErrPitTrap}
}

// overrideReg replaces a node's reg property with <base width> (hex) + cmt,
// used for the aic+pit combined region.
func overrideReg(n *dts.Node, base, width uint64, cmt string) {
	reg := &dts.Prop{
		Name:   "reg",
		Values: []dts.Value{dts.Cells{Nums: []uint64{base, width}, Hex: true}},
		Cmt:    cmt,
	}
	for i, p := range n.Props {
		if p.Name == "reg" {
			n.Props[i] = reg
			return
		}
	}
	n.Props = append(n.Props, reg)
}

// sortedNamedKeys returns the named-irq map keys in ascending order.
func sortedNamedKeys(m map[string]*design.IRQEntry) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}

// lc lower-cases and trims a class name (matching elaborate.lc).
func lc(s string) string { return strings.ToLower(strings.TrimSpace(s)) }
