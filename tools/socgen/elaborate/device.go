package elaborate

import (
	"errors"
	"fmt"

	"github.com/j-core/jcore-soc/tools/socgen/board"
	"github.com/j-core/jcore-soc/tools/socgen/design"
)

// Devices resolves the device classes actually instantiated by the board into
// a *Resolution (per-device: entity/arch, registers, name, effective generics).
// Classes defined in the board's YAML but not referenced by any device are
// skipped — they may have empty/absent entity fields and would produce spurious
// errors. Best-effort; never panics.
func Devices(b *board.Board) (*Resolution, error) {
	res := &Resolution{Classes: map[string]*ResolvedClass{}}
	if b == nil || b.Design == nil || b.Library == nil {
		return res, nil
	}
	errs := make([]error, 0, len(b.Design.Devices)+1)
	// Resolve only the classes actually instantiated by a device.
	seen := map[string]bool{}
	for _, dev := range b.Design.Devices {
		key := lc(dev.Class)
		if seen[key] {
			continue
		}
		seen[key] = true
		dc, ok := b.Design.DeviceClasses[dev.Class]
		if !ok {
			// try a case-insensitive match against the class map
			for cn, c := range b.Design.DeviceClasses {
				if lc(cn) == key {
					dc, ok = c, true
					break
				}
			}
		}
		if !ok {
			errs = append(errs, &ResolveError{Kind: ErrUnknownClass, Ctx: fmt.Sprintf("device %q", dev.Name), Name: dev.Class,
				Detail: fmt.Sprintf("unknown class %q", dev.Class)})
			continue
		}
		rc, err := resolveClass(dev.Class, dc, b.Library)
		errs = append(errs, err)
		res.Classes[key] = rc
	}
	devs, err := resolveDevices(b.Design, res.Classes)
	res.Devices = devs
	errs = append(errs, err)
	return res, errors.Join(errs...)
}

// resolveDevices assigns unique names and merges effective generics for each
// device instance (faithful to assign-device-names + the generic merge).
func resolveDevices(d *design.Design, classes map[string]*ResolvedClass) ([]*ResolvedDevice, error) {
	errs := make([]error, 0, len(d.Devices))
	// duplicate explicit-name check + class counts
	classCount := map[string]int{}
	nameCount := map[string]int{}
	for _, dev := range d.Devices {
		classCount[lc(dev.Class)]++
		if dev.Name != "" {
			nameCount[dev.Name]++
		}
	}
	for n, c := range nameCount {
		if c > 1 {
			errs = append(errs, &ResolveError{Kind: ErrDuplicateName, Ctx: fmt.Sprintf("device %q", n), Name: n,
				Detail: fmt.Sprintf("multiple devices named %q; names must be unique", n)})
		}
	}
	used := map[string]bool{}
	for n := range nameCount {
		used[n] = true
	}

	out := make([]*ResolvedDevice, 0, len(d.Devices))
	for _, dev := range d.Devices {
		name := dev.Name
		if name == "" {
			if classCount[lc(dev.Class)] == 1 && !used[dev.Class] {
				name = dev.Class
			} else {
				name = genUniqueName(dev.Class, used)
			}
		}
		used[name] = true

		rc := classes[lc(dev.Class)]
		generics := map[string]design.Value{}
		if rc != nil {
			for k, v := range rc.Generics {
				generics[k] = v
			}
		}
		for k, v := range dev.Generics {
			generics[k] = v // instance overrides class
		}
		// every effective generic must exist on the entity
		if rc != nil && rc.Entity != nil {
			gset := map[string]bool{}
			for _, g := range rc.Entity.Generics {
				gset[lc(g.Name)] = true
			}
			for k := range generics {
				if !gset[lc(k)] {
					errs = append(errs, &ResolveError{Kind: ErrUnknownGeneric, Ctx: fmt.Sprintf("device %q", name), Name: k,
						Detail: fmt.Sprintf("unknown generic %q for entity %q", k, rc.Entity.Name)})
				}
			}
		}
		var base *uint64
		if dev.BaseAddr != nil {
			u := uint64(*dev.BaseAddr)
			base = &u
		}
		out = append(out, &ResolvedDevice{Name: name, Class: dev.Class, Generics: generics, BaseAddr: base})
	}
	return out, errors.Join(errs...)
}

func genUniqueName(class string, used map[string]bool) string {
	for i := 0; ; i++ {
		n := fmt.Sprintf("%s%d", class, i)
		if !used[n] {
			return n
		}
	}
}
