package elaborate

import (
	"errors"
	"fmt"
	"sort"

	"github.com/j-core/jcore-soc/tools/socgen/design"
	"github.com/j-core/jcore-soc/tools/socgen/iface"
)

// resolveEntity resolves a single top-entity or padring-entity. It binds the
// named entity (defaulting to the map key when no `entity:` is given), chooses
// its architecture/configuration via the shared chooseArch, and builds its ports
// via the shared buildPorts. kind is "top" or "padring" (error-context only).
// Best-effort: a port-less, nil-entity ResolvedEntity is returned on bind failure.
func resolveEntity(kind, name string, te *design.TopEntity, lib *iface.Library, merge map[string]string) (*ResolvedEntity, error) {
	re := &ResolvedEntity{Name: name}
	re.Generics = te.Generics
	entityName := te.Entity
	if lc(entityName) == "" {
		entityName = name
	}
	ctx := fmt.Sprintf("%s-entity %q", kind, name)
	// hardErr is intentionally not checked here: unlike resolveClass (which skips
	// register resolution on a hard bind failure), a top/padring entity has no such
	// dependent step, and buildPorts already nil-guards on ent.
	ent, arch, cfg, _, err := chooseArch(ctx, entityName, te.Architecture, te.Configuration, lib)
	re.Entity, re.ArchName, re.Config = ent, arch, cfg
	env := genericEnv(te.Generics, ent)
	re.Ports = buildPorts(name, ent, te.Ports, env, merge, true, lib)
	return re, err
}

// resolveEntities resolves a whole top-entities or padring-entities map in sorted
// key order (deterministic error accumulation).
func resolveEntities(kind string, ents map[string]*design.TopEntity, lib *iface.Library, merge map[string]string) (map[string]*ResolvedEntity, error) {
	errs := make([]error, 0, len(ents))
	out := map[string]*ResolvedEntity{}
	for _, name := range sortedTopKeys(ents) {
		re, err := resolveEntity(kind, name, ents[name], lib, merge)
		errs = append(errs, err)
		out[name] = re
	}
	return out, errors.Join(errs...)
}

func sortedTopKeys(m map[string]*design.TopEntity) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}
