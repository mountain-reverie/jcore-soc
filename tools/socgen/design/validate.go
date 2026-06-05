package design

import (
	"fmt"
	"sort"
	"strings"

	"github.com/j-core/jcore-soc/tools/socgen/iface"
)

// Validate cross-checks a Design against an iface.Library: device classes
// resolve, class/top/padring entities resolve (directly or via a configuration),
// and each generic/port key exists on the resolved entity interface. It returns
// one error per unresolved reference (best-effort; never panics).
func Validate(d *Design, lib *iface.Library) []error {
	var errs []error

	resolveEntity := func(entityName, configName, ctx string) *iface.Entity {
		if configName != "" {
			cfg, ok := lib.Configuration(configName)
			if !ok {
				errs = append(errs, fmt.Errorf("%s: configuration %q not found", ctx, configName))
				return nil
			}
			if cfg.Arch != "" {
				archs := lib.ArchitecturesOf(cfg.Entity)
				if len(archs) > 0 {
					found := false
					for _, a := range archs {
						if strings.EqualFold(a.Name, cfg.Arch) {
							found = true
							break
						}
					}
					if !found {
						errs = append(errs, fmt.Errorf("%s: configuration %q architecture %q not found for entity %q", ctx, configName, cfg.Arch, cfg.Entity))
					}
				}
			}
			entityName = cfg.Entity
		}
		if entityName == "" {
			return nil
		}
		e, ok := lib.Entity(entityName)
		if !ok {
			errs = append(errs, fmt.Errorf("%s: entity %q not found", ctx, entityName))
			return nil
		}
		return e
	}

	checkIface := func(e *iface.Entity, generics, ports map[string]Value, ctx string) {
		if e == nil {
			return
		}
		gset := nameSet(len(e.Generics))
		for _, g := range e.Generics {
			gset[strings.ToLower(g.Name)] = struct{}{}
		}
		pset := nameSet(len(e.Ports))
		for _, p := range e.Ports {
			pset[strings.ToLower(p.Name)] = struct{}{}
		}
		for _, k := range sortedKeys(generics) {
			if _, ok := gset[strings.ToLower(k)]; !ok {
				errs = append(errs, fmt.Errorf("%s: generic %q not on entity %q", ctx, k, e.Name))
			}
		}
		for _, k := range sortedKeys(ports) {
			if _, ok := pset[strings.ToLower(k)]; !ok {
				errs = append(errs, fmt.Errorf("%s: port %q not on entity %q", ctx, k, e.Name))
			}
		}
	}

	for _, dev := range d.Devices {
		cls, ok := d.DeviceClasses[dev.Class]
		if !ok {
			errs = append(errs, fmt.Errorf("device %q: unknown class %q", devID(dev), dev.Class))
			continue
		}
		ctx := "device " + devID(dev)
		e := resolveEntity(cls.Entity, cls.Configuration, ctx)
		checkIface(e, dev.Generics, dev.Ports, ctx)
	}
	for _, name := range sortedEntityKeys(d.TopEntities) {
		te := d.TopEntities[name]
		ctx := "top-entity " + name
		e := resolveEntity(te.Entity, te.Configuration, ctx)
		checkIface(e, te.Generics, te.Ports, ctx)
	}
	for _, name := range sortedEntityKeys(d.PadringEntities) {
		te := d.PadringEntities[name]
		ctx := "padring-entity " + name
		e := resolveEntity(te.Entity, te.Configuration, ctx)
		checkIface(e, te.Generics, te.Ports, ctx)
	}
	return errs
}

func devID(d *Device) string {
	if d.Name != "" {
		return d.Name
	}
	return d.Class
}

func nameSet(n int) map[string]struct{} { return make(map[string]struct{}, n) }

func sortedEntityKeys(m map[string]*TopEntity) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}

func sortedKeys(m map[string]Value) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks) // deterministic error order
	return ks
}
