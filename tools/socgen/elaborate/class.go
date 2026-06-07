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
// or configuration (via the shared chooseArch), then resolves its registers.
// Appends errors; returns a best-effort *ResolvedClass.
func resolveClass(name string, dc *design.DeviceClass, lib *iface.Library, errs []error) (*ResolvedClass, []error) {
	rc := &ResolvedClass{Name: name, Generics: dc.Generics}
	ent, arch, cfg, hardErr, errs := chooseArch(fmt.Sprintf("class %q", name), dc.Entity, dc.Architecture, dc.Configuration, lib, errs)
	rc.Entity, rc.ArchName, rc.Config = ent, arch, cfg
	if !hardErr {
		// A hard failure (entity/config/arch not found, or arch+config mismatch)
		// skips register resolution, exactly as the original early returns did.
		rc.Regs, rc.LeftAddrBit, rc.RegRange, errs = resolveRegs(name, dc, errs)
	}
	return rc, errs
}

// chooseArch binds entityName to an entity and selects its architecture or
// configuration (faithful to choose-device-arch). ctx is an error-message label
// (e.g. `class "uartlite"` or `top-entity "cpus"`). It returns hardErr=true on a
// hard failure — entity / configuration / architecture not found, or arch+config
// mismatch — so the caller can skip dependent work. The soft cases (no
// architecture, or an ambiguous choice among many) append an error but return the
// bound entity with hardErr=false, matching the original fall-through behavior.
func chooseArch(ctx, entityName, archName, configName string, lib *iface.Library, errs []error) (*iface.Entity, string, *iface.Configuration, bool, []error) {
	if lib == nil {
		return nil, "", nil, true, append(errs, fmt.Errorf("%s: no library", ctx))
	}
	entityID := lc(entityName)
	ent, ok := lib.Entity(entityID)
	if !ok {
		return nil, "", nil, true, append(errs, fmt.Errorf("%s: unable to map to entity %q", ctx, entityName))
	}

	archs := lib.ArchitecturesOf(entityID)
	configID := lc(configName)
	archID := lc(archName)

	var cfg *iface.Configuration
	if configID != "" {
		c, ok := lib.Configuration(configID)
		if !ok {
			return ent, "", nil, true, append(errs, fmt.Errorf("%s: configuration %q of entity %q not found", ctx, configName, entityName))
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
			return ent, "", nil, true, append(errs, fmt.Errorf("%s: architecture %q of entity %q not found", ctx, archName, entityName))
		}
	}

	switch {
	case cfg != nil && arch != nil:
		if lc(arch.Name) != lc(cfg.Arch) {
			return ent, "", nil, true, append(errs, fmt.Errorf("%s: architecture %q and configuration %q (arch %q) mismatch; set only configuration", ctx, archName, configName, cfg.Arch))
		}
		return ent, cfg.Arch, cfg, false, errs
	case cfg != nil:
		return ent, cfg.Arch, cfg, false, errs
	case arch != nil:
		return ent, arch.Name, nil, false, errs
	case len(archs) == 1:
		return ent, archs[0].Name, nil, false, errs
	case len(archs) == 0:
		return ent, "", nil, false, append(errs, fmt.Errorf("%s: unable to find any architecture for entity %q", ctx, entityName))
	default:
		return ent, "", nil, false, append(errs, fmt.Errorf("%s: unable to find single architecture for entity %q (%d found)", ctx, entityName, len(archs)))
	}
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
