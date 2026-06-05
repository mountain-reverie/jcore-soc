package elaborate

import (
	"fmt"
	"math"
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
	rc.Regs, rc.LeftAddrBit, rc.RegRange, errs = resolveRegs(name, dc, errs)
	return rc, errs
}

func resolveRegs(class string, dc *design.DeviceClass, errs []error) ([]*ResolvedReg, int, [2]int, []error) {
	addr := 0
	var regs []*ResolvedReg
	for _, r := range dc.Regs {
		width := 4
		if r.Width != nil {
			width = *r.Width
		}
		a := addr
		if r.Addr != nil {
			a = *r.Addr
		}
		typ := r.Type
		if typ == "" {
			typ = "fixed"
		}
		rr := &ResolvedReg{
			Name:      lc(r.Name),
			Addr:      a,
			Width:     width,
			ByteRange: [2]int{a, a + width - 1},
			Mode:      r.Mode,
			Type:      typ,
		}
		regs = append(regs, rr)
		addr = a + width
	}
	if len(regs) == 0 {
		return nil, 0, [2]int{}, errs
	}
	// per-class register overlap (local check; cross-device is P4e)
	for i := 0; i < len(regs); i++ {
		for j := i + 1; j < len(regs); j++ {
			if overlaps(regs[i].ByteRange, regs[j].ByteRange) {
				errs = append(errs, fmt.Errorf("class %q: register %q overlaps %q", class, regs[i].Name, regs[j].Name))
			}
		}
	}
	maxAddr := 0
	low, high := regs[0].ByteRange[0], regs[0].ByteRange[1]
	for _, r := range regs {
		if e := r.Addr + r.Width; e > maxAddr {
			maxAddr = e
		}
		if r.ByteRange[0] < low {
			low = r.ByteRange[0]
		}
		if r.ByteRange[1] > high {
			high = r.ByteRange[1]
		}
	}
	if maxAddr < 1 {
		maxAddr = 1
	}
	required := int(math.Ceil(math.Log2(float64(maxAddr)))) - 1
	leftBit := required
	if dc.LeftAddrBit > 0 {
		if dc.LeftAddrBit < required {
			errs = append(errs, fmt.Errorf("class %q: left-addr-bit %d too small for registers, must be at least %d", class, dc.LeftAddrBit, required))
		}
		if dc.LeftAddrBit > required {
			leftBit = dc.LeftAddrBit
		}
	}
	regRange := [2]int{low - low%4, ((high / 4) + 1) * 4} // align low down, high up to 4
	return regs, leftBit, regRange, errs
}

func overlaps(a, b [2]int) bool { return a[0] <= b[1] && b[0] <= a[1] }
