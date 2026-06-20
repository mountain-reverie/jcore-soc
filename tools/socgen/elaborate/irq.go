package elaborate

import (
	"errors"
	"fmt"
	"sort"

	"github.com/j-core/jcore-soc/tools/socgen/design"
)

var (
	// ErrIRQNoPort: a device with a single irq has no KindIRQ output port.
	ErrIRQNoPort = errors.New("device has no irq port")
	// ErrIRQBadCPU: an irq references a negative cpu.
	ErrIRQBadCPU = errors.New("irq cpu out of range")
	// ErrIRQBadPath: an irq path (line number) is outside 0..7.
	ErrIRQBadPath = errors.New("irq path out of range")
	// ErrIRQNamedPort: a named irq port does not exist on the device entity.
	ErrIRQNamedPort = errors.New("named irq port not found on entity")
	// ErrIRQVectorMismatch: an OR-group's tuples disagree on the vector number.
	ErrIRQVectorMismatch = errors.New("or-combined irqs disagree on vector")
	// ErrIRQAicCoverage: the set of aic cpus is not {0..N-1}.
	ErrIRQAicCoverage = errors.New("aic cpus must cover {0..N-1}")
)

// IRQError reports an AIC1 interrupt-wiring resolution failure.
type IRQError struct {
	Kind   error
	Device string // subject device ("" for coverage errors)
	Port   string // subject port, where applicable
	Detail string // formatted specifics
}

func (e *IRQError) Error() string {
	// subject names the device (and port, where applicable) the error is about,
	// mirroring the context prefix used by ResolveError/AddrError.
	subject := ""
	switch {
	case e.Device != "" && e.Port != "":
		subject = fmt.Sprintf("device %q port %q", e.Device, e.Port)
	case e.Device != "":
		subject = fmt.Sprintf("device %q", e.Device)
	}
	switch {
	case subject != "" && e.Detail != "":
		return fmt.Sprintf("%s: %v: %s", subject, e.Kind, e.Detail)
	case subject != "":
		return fmt.Sprintf("%s: %v", subject, e.Kind)
	case e.Detail != "":
		return fmt.Sprintf("%v: %s", e.Kind, e.Detail)
	default:
		return e.Kind.Error()
	}
}
func (e *IRQError) Unwrap() error { return e.Kind }

// irqTuple is one resolved device interrupt: device port -> {cpu, path, vector}.
type irqTuple struct {
	dev, port         string
	cpu, path, vector int
}

// cpuOf returns a device's cpu (default 0).
func cpuOf(dev *design.Device) int {
	if dev.CPU != nil {
		return *dev.CPU
	}
	return 0
}

// resolveIRQ builds the AIC1 interrupt-wiring model (P5e), faithful to irq.clj's
// AIC1Plugin. Returns nil if the design has no aic. It discards validation
// problems; Elaborate calls buildIRQ directly and joins those into its error set
// (matching the package's typed-error model). Retained as a test facade.
func resolveIRQ(res *Resolution, d *design.Design) *IRQModel {
	m, _ := buildIRQ(res, d)
	return m
}

// buildIRQ constructs the IRQ model and accumulates validation errors.
func buildIRQ(res *Resolution, d *design.Design) (*IRQModel, []error) {
	if d == nil {
		return nil, nil
	}
	var errs []error
	// index resolved devices by name for KindIRQ / entity-port lookups.
	rdByName := map[string]*ResolvedDevice{}
	for _, rd := range res.Devices {
		rdByName[rd.Name] = rd
	}
	// A design device's Name is the as-written value, which is empty when the
	// instance relied on name-defaulting (class-derived, e.g. gpio/cache_ctrl).
	// The resolved devices (res.Devices) carry the effective name and are built
	// 1:1 in design order, so the resolved name at the matching index is the
	// device's true instance name; fall back to the as-written name otherwise.
	effName := func(i int, dev *design.Device) string {
		if i < len(res.Devices) {
			return res.Devices[i].Name
		}
		return dev.Name
	}

	// aicByCPU: cpu -> aic device name.
	aicByCPU := map[int]string{}
	// aics whose design explicitly maps irq_i (board owns the irq vector, e.g. a
	// raw-pin IRQ source) — their irq_i is NOT auto-wired to irqs<cpu> below.
	aicExplicitIrqI := map[string]bool{}
	for i, dev := range d.Devices {
		if lc(dev.Class) != "aic" {
			continue
		}
		name := effName(i, dev)
		cpu := cpuOf(dev)
		if cpu < 0 {
			errs = append(errs, &IRQError{Kind: ErrIRQBadCPU, Device: name, Detail: fmt.Sprintf("cpu %d", cpu)})
			continue
		}
		aicByCPU[cpu] = name
		if _, ok := dev.Ports["irq_i"]; ok {
			aicExplicitIrqI[name] = true
		}
	}
	if len(aicByCPU) == 0 {
		return nil, errs
	}

	// aic cpus must cover {0..N-1}.
	aicCPUs := sortedIntKeys(aicByCPU)
	for i, cpu := range aicCPUs {
		if cpu != i {
			errs = append(errs, &IRQError{Kind: ErrIRQAicCoverage, Detail: fmt.Sprintf("cpus %v", aicCPUs)})
			break
		}
	}

	// gather one tuple per device interrupt line.
	var tuples []irqTuple
	for i, dev := range d.Devices {
		if dev.IRQ == nil {
			continue
		}
		name := effName(i, dev)
		rd := rdByName[name]
		switch {
		case dev.IRQ.Int != nil:
			irq := *dev.IRQ.Int
			port := irqPortName(rd)
			if port == "" {
				errs = append(errs, &IRQError{Kind: ErrIRQNoPort, Device: name})
				continue
			}
			tuples = append(tuples, irqTuple{dev: name, port: port, cpu: cpuOf(dev), path: irq, vector: 0x11 + irq})
		case dev.IRQ.Named != nil:
			for _, port := range sortedStrKeys(dev.IRQ.Named) {
				e := dev.IRQ.Named[port]
				if !entityHasPort(res, dev, port) {
					errs = append(errs, &IRQError{Kind: ErrIRQNamedPort, Device: name, Port: port})
					continue
				}
				tuples = append(tuples, irqTuple{dev: name, port: port, cpu: e.CPU, path: e.IRQ, vector: 0x11 + e.IRQ})
			}
		}
	}

	m := &IRQModel{
		PortOverrides: map[string]map[string]string{},
		VectorNumbers: map[string][8]int{},
	}

	// irqs<cpu> signal + empty vector table per aic-cpu, sorted by cpu.
	for _, cpu := range aicCPUs {
		m.Signals = append(m.Signals, IRQSignal{Name: fmt.Sprintf("irqs%d", cpu), Width: 8})
		m.VectorNumbers[aicByCPU[cpu]] = [8]int{}
	}

	// group tuples by {cpu, path}, iterate in sorted (cpu, path) order.
	type key struct{ cpu, path int }
	groups := map[key][]irqTuple{}
	for _, t := range tuples {
		k := key{t.cpu, t.path}
		groups[k] = append(groups[k], t)
	}
	keys := make([]key, 0, len(groups))
	for k := range groups {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].cpu != keys[j].cpu {
			return keys[i].cpu < keys[j].cpu
		}
		return keys[i].path < keys[j].path
	})

	for _, k := range keys {
		g := groups[k]
		aic, hasAic := aicByCPU[k.cpu]
		if k.cpu < 0 {
			errs = append(errs, &IRQError{Kind: ErrIRQBadCPU, Device: g[0].dev, Detail: fmt.Sprintf("cpu %d", k.cpu)})
		}
		if k.path < 0 || k.path > 7 {
			errs = append(errs, &IRQError{Kind: ErrIRQBadPath, Device: g[0].dev, Detail: fmt.Sprintf("path %d", k.path)})
		}
		if !hasAic {
			// no aic on this cpu -> each port is left open.
			for _, t := range g {
				setOverride(m, t.dev, t.port, "open")
			}
			continue
		}
		// record the vector for this path on the aic (validate group agreement).
		if k.path >= 0 && k.path <= 7 {
			vec := m.VectorNumbers[aic]
			vec[k.path] = g[0].vector
			m.VectorNumbers[aic] = vec
		}
		target := fmt.Sprintf("irqs%d(%d)", k.cpu, k.path)
		if len(g) == 1 {
			setOverride(m, g[0].dev, g[0].port, target)
			continue
		}
		// OR-combine: per-device source signal, each port -> its source.
		var sources []string
		for i, t := range g {
			if t.vector != g[0].vector {
				errs = append(errs, &IRQError{
					Kind:   ErrIRQVectorMismatch,
					Device: t.dev,
					Detail: fmt.Sprintf("vector %#x != %#x at irqs%d(%d)", t.vector, g[0].vector, k.cpu, k.path),
				})
			}
			// i is bounded by devices sharing one IRQ path; >25 would yield a
			// non-alpha suffix, but a real SoC never approaches that.
			src := fmt.Sprintf("irq_%d_%d_%s", k.cpu, k.path, string(rune('a'+i)))
			m.Signals = append(m.Signals, IRQSignal{Name: src, Width: 0})
			setOverride(m, t.dev, t.port, src)
			sources = append(sources, src)
		}
		m.OrAssigns = append(m.OrAssigns, IRQOrAssign{Target: target, Sources: sources})
	}

	// each aic's irq_i input is wired to its own cpu's irqs<cpu>, unless the
	// design maps irq_i explicitly (then the board's mapping wins).
	for _, cpu := range aicCPUs {
		if aicExplicitIrqI[aicByCPU[cpu]] {
			continue
		}
		setOverride(m, aicByCPU[cpu], "irq_i", fmt.Sprintf("irqs%d", cpu))
	}

	return m, errs
}

// irqPortName returns the name of a device's KindIRQ output port ("" if none).
func irqPortName(rd *ResolvedDevice) string {
	if rd == nil {
		return ""
	}
	for _, p := range rd.Ports {
		if p.Kind == KindIRQ {
			return p.Name
		}
	}
	return ""
}

// entityHasPort reports whether the device's bound entity declares a port named
// port (best-effort: true when the entity, or its port list, is unavailable, so
// an unbound/partially-modelled entity does not spuriously fail the check).
func entityHasPort(res *Resolution, dev *design.Device, port string) bool {
	rc := res.Classes[lc(dev.Class)]
	if rc == nil || rc.Entity == nil || len(rc.Entity.Ports) == 0 {
		return true
	}
	for _, p := range rc.Entity.Ports {
		if p.Name == port {
			return true
		}
	}
	return false
}

// setOverride records devName.portName -> text in the override table.
func setOverride(m *IRQModel, devName, portName, text string) {
	if m.PortOverrides[devName] == nil {
		m.PortOverrides[devName] = map[string]string{}
	}
	m.PortOverrides[devName][portName] = text
}

// sortedIntKeys returns the integer keys of m in ascending order.
func sortedIntKeys[V any](m map[int]V) []int {
	ks := make([]int, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Ints(ks)
	return ks
}

// sortedStrKeys returns the string keys of m in ascending order.
func sortedStrKeys[V any](m map[string]V) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}
