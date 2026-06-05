package elaborate

import (
	"fmt"

	"github.com/j-core/jcore-soc/tools/socgen/board"
	"github.com/j-core/jcore-soc/tools/socgen/design"
)

// Devices resolves every spec device of a loaded board into a *Resolution
// (per-device: entity/arch, registers, name, effective generics). Best-effort;
// never panics.
func Devices(b *board.Board) (*Resolution, []error) {
	res := &Resolution{Classes: map[string]*ResolvedClass{}}
	var errs []error
	if b == nil || b.Design == nil {
		return res, errs
	}
	for name, dc := range b.Design.DeviceClasses {
		var rc *ResolvedClass
		rc, errs = resolveClass(name, dc, b.Library, errs)
		res.Classes[lc(name)] = rc
	}
	res.Devices, errs = resolveDevices(b.Design, res.Classes, errs)
	return res, errs
}

// resolveDevices assigns unique names and merges effective generics for each
// device instance (faithful to assign-device-names + the generic merge).
func resolveDevices(d *design.Design, classes map[string]*ResolvedClass, errs []error) ([]*ResolvedDevice, []error) {
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
			errs = append(errs, fmt.Errorf("multiple devices named %q; names must be unique", n))
		}
	}
	used := map[string]bool{}
	for n := range nameCount {
		used[n] = true
	}

	var out []*ResolvedDevice
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
					errs = append(errs, fmt.Errorf("device %q: unknown generic %q for entity %q", name, k, rc.Entity.Name))
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
	return out, errs
}

func genUniqueName(class string, used map[string]bool) string {
	for i := 0; ; i++ {
		n := fmt.Sprintf("%s%d", class, i)
		if !used[n] {
			return n
		}
	}
}
