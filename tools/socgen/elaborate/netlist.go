package elaborate

import (
	"errors"
	"sort"

	"github.com/j-core/jcore-soc/tools/socgen/board"
	"github.com/j-core/jcore-soc/tools/socgen/design"
)

// Elaborate runs device resolution (P4b) then builds the net-list (P4c):
// per-device ports, gather global signals, apply zero-signals.
func Elaborate(b *board.Board) (*Resolution, error) {
	res, err := Devices(b)
	if b == nil || b.Design == nil {
		return res, err
	}
	errs := []error{err}
	merge := reverseMerge(b.Design.MergeSignals)
	// resolve each device's ports
	// res.Devices[i] corresponds to b.Design.Devices[i] (resolveDevices iterates
	// d.Devices in order, appending one ResolvedDevice per entry — no filtering).
	// Use index-based association so that auto-named devices (Name=="") still get
	// their instance port overrides applied; name-based lookup would never match
	// because the auto-generated resolved name (e.g. "gpio0") != "" in Design.Devices.
	for i, dev := range res.Devices {
		rc := res.Classes[lc(dev.Class)]
		if rc == nil {
			continue
		}
		env := genericEnv(dev.Generics, rc.Entity)
		spec := map[string]design.Value{}
		if dc, ok := b.Design.DeviceClasses[dev.Class]; ok {
			for k, v := range dc.Ports {
				spec[k] = v
			}
		}
		// instance port overrides: zip by index (res.Devices[i] <-> Design.Devices[i])
		if i < len(b.Design.Devices) {
			for k, v := range b.Design.Devices[i].Ports {
				spec[k] = v
			}
		}
		dev.Ports = buildPorts(dev.Name, rc.Entity, spec, env, merge)
	}
	topEnts, terr := resolveEntities("top", b.Design.TopEntities, b.Library, merge)
	res.TopEntities = topEnts
	errs = append(errs, terr)
	padEnts, perr := resolveEntities("padring", b.Design.PadringEntities, b.Library, merge)
	res.PadringEntities = padEnts
	errs = append(errs, perr)
	res.Signals = gatherSignals(res)
	// Pins resolve AFTER gather (bare-signal direction reads existing drivers) and
	// BEFORE zero-signals (so a pin-driven signal isn't given a synthetic driver).
	res.Pins = resolvePins(b.Design, res.Signals)
	applyZeroSignals(res.Signals, b.Design.ZeroSignals)
	errs = append(errs, validateSignals(res.Signals))
	errs = append(errs, validateAddresses(res))
	return res, errors.Join(errs...)
}

// gatherSignals groups KindSignal ports (across devices, top entities and padring
// entities) by global-signal name. Pins and zero-signals are joined afterwards by
// Elaborate (order matters — see Elaborate).
func gatherSignals(res *Resolution) map[string]*Signal {
	sigs := map[string]*Signal{}
	for _, dev := range res.Devices {
		addPortsToSignals(sigs, Context{Kind: "device", ID: dev.Name}, dev.Ports)
	}
	for _, name := range sortedEntityNames(res.TopEntities) {
		addPortsToSignals(sigs, Context{Kind: "top", ID: name}, res.TopEntities[name].Ports)
	}
	for _, name := range sortedEntityNames(res.PadringEntities) {
		addPortsToSignals(sigs, Context{Kind: "padring", ID: name}, res.PadringEntities[name].Ports)
	}
	return sigs
}

// applyZeroSignals adds a synthetic :out driver to each listed signal that exists
// but is undriven. No-op for names not present in the net-list.
func applyZeroSignals(sigs map[string]*Signal, zero []string) {
	for _, z := range zero {
		s := sigs[z]
		if s == nil {
			continue
		}
		driven := false
		for _, pr := range s.Ports {
			if pr.Dir == dirOut {
				driven = true
				break
			}
		}
		if !driven {
			s.Ports = append(s.Ports, &SignalPortRef{
				Context:  Context{Kind: "zero", ID: "_zero"},
				PortName: z,
				Dir:      dirOut,
				Type:     s.Type,
			})
		}
	}
}

// addPortsToSignals records each KindSignal port under its global-signal name.
func addPortsToSignals(sigs map[string]*Signal, ctx Context, ports []*ResolvedPort) {
	for _, p := range ports {
		if p.Kind != KindSignal || p.GlobalSignal == "" {
			continue
		}
		s := sigs[p.GlobalSignal]
		if s == nil {
			s = &Signal{Name: p.GlobalSignal, Type: p.Type}
			sigs[p.GlobalSignal] = s
		}
		s.Ports = append(s.Ports, &SignalPortRef{
			Context:  ctx,
			PortName: p.Name,
			Dir:      p.Dir,
			Type:     p.Type,
		})
	}
}

func sortedEntityNames(m map[string]*ResolvedEntity) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}
