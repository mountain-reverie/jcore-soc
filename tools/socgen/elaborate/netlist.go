package elaborate

import (
	"github.com/j-core/jcore-soc/tools/socgen/board"
	"github.com/j-core/jcore-soc/tools/socgen/design"
)

// Elaborate runs device resolution (P4b) then builds the net-list (P4c):
// per-device ports, gather global signals, apply zero-signals.
func Elaborate(b *board.Board) (*Resolution, []error) {
	res, errs := Devices(b)
	if b == nil || b.Design == nil {
		return res, errs
	}
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
	res.Signals, errs = gatherSignals(res, b.Design.ZeroSignals, errs)
	errs = validateSignals(res.Signals, errs)
	return res, errs
}

// gatherSignals groups KindSignal device ports by global-signal name and adds
// synthetic zero-signal drivers for undriven listed signals.
func gatherSignals(res *Resolution, zero []string, errs []error) (map[string]*Signal, []error) {
	sigs := map[string]*Signal{}
	for _, dev := range res.Devices {
		for _, p := range dev.Ports {
			if p.Kind != KindSignal || p.GlobalSignal == "" {
				continue
			}
			s := sigs[p.GlobalSignal]
			if s == nil {
				s = &Signal{Name: p.GlobalSignal, Type: p.Type}
				sigs[p.GlobalSignal] = s
			}
			s.Ports = append(s.Ports, &SignalPortRef{
				Context:  Context{Kind: "device", ID: dev.Name},
				PortName: p.Name,
				Dir:      p.Dir,
				Type:     p.Type,
			})
		}
	}
	// zero-signals: add a synthetic :out driver to an undriven listed signal
	for _, z := range zero {
		s := sigs[z]
		if s == nil {
			continue // a zero-signal that no port references — nothing to drive (or P4d)
		}
		driven := false
		for _, pr := range s.Ports {
			if pr.Dir == "out" {
				driven = true
				break
			}
		}
		if !driven {
			s.Ports = append(s.Ports, &SignalPortRef{
				Context:  Context{Kind: "zero", ID: "_zero"},
				PortName: z,
				Dir:      "out",
				Type:     s.Type,
			})
		}
	}
	return sigs, errs
}
