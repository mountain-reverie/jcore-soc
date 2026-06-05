package elaborate

import (
	"fmt"
	"strings"

	"github.com/j-core/jcore-soc/tools/socgen/design"
	"github.com/j-core/jcore-soc/tools/socgen/iface"
)

func lc(s string) string { return strings.ToLower(strings.TrimSpace(s)) }

// resolveClass binds a device class to its entity and chooses its architecture
// or configuration (faithful to choose-device-arch). Registers/left-addr-bit are
// added in Task 2. Appends errors; returns a best-effort *ResolvedClass.
func resolveClass(name string, dc *design.DeviceClass, lib *iface.Library, errs []error) (*ResolvedClass, []error) {
	rc := &ResolvedClass{Name: name, Generics: dc.Generics}
	entityID := lc(dc.Entity)
	ent, ok := lib.Entity(entityID)
	if !ok {
		return rc, append(errs, fmt.Errorf("class %q: unable to map to entity %q", name, dc.Entity))
	}
	rc.Entity = ent

	archs := lib.ArchitecturesOf(entityID)
	configID := lc(dc.Configuration)
	archID := lc(dc.Architecture)

	var cfg *iface.Configuration
	if configID != "" {
		c, ok := lib.Configuration(configID)
		if !ok {
			return rc, append(errs, fmt.Errorf("class %q: configuration %q of entity %q not found", name, dc.Configuration, dc.Entity))
		}
		cfg = c
	}
	var arch *iface.Architecture
	if archID != "" {
		for _, a := range archs {
			if lc(a.Name) == archID {
				arch = a
				break
			}
		}
		if arch == nil {
			return rc, append(errs, fmt.Errorf("class %q: architecture %q of entity %q not found", name, dc.Architecture, dc.Entity))
		}
	}

	switch {
	case cfg != nil && arch != nil:
		if lc(arch.Name) != lc(cfg.Arch) {
			return rc, append(errs, fmt.Errorf("class %q: architecture %q and configuration %q (arch %q) mismatch; set only configuration", name, dc.Architecture, dc.Configuration, cfg.Arch))
		}
		rc.Config, rc.ArchName = cfg, cfg.Arch
	case cfg != nil:
		rc.Config, rc.ArchName = cfg, cfg.Arch
	case arch != nil:
		rc.ArchName = arch.Name
	case len(archs) == 1:
		rc.ArchName = archs[0].Name
	case len(archs) == 0:
		errs = append(errs, fmt.Errorf("class %q: unable to find any architecture for entity %q", name, dc.Entity))
	default:
		errs = append(errs, fmt.Errorf("class %q: unable to find single architecture for entity %q (%d found)", name, dc.Entity, len(archs)))
	}
	return rc, errs
}
