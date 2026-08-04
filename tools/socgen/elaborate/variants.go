package elaborate

import (
	"fmt"
	"path/filepath"
	"runtime"
	"sync"

	"github.com/BurntSushi/toml"
)

// cpuVariant mirrors one [<name>] table in components/cpu/variants.toml --
// the submodule's single authoritative CPU variant table (generics, decoder
// overlay, extra sources, VHDL configuration file). jcore-soc does not
// duplicate that knowledge: it reads the file directly, because the
// submodule's TOML parser lives under an `internal/` package
// (components/cpu/decode/gen-go/internal/variants) that Go's import rules
// forbid a separate module from importing, and adding a `replace` directive
// into the submodule would couple socgen's build to submodule checkout,
// breaking CI clones that don't --recurse-submodules.
type cpuVariant struct {
	Generics   map[string]string `toml:"generics"`
	Overlay    string            `toml:"overlay"`
	ExtraFiles []string          `toml:"extra_files"`
	ConfigFile string            `toml:"config_file"`
}

// Root is the repository root (the directory containing targets/boards and
// components/cpu), set by cmd/socgen from its existing -root flag via
// SetRoot. When unset (e.g. `go test ./elaborate/...` run directly), it
// defaults to the location implied by this source file's own path in the
// tree, three levels above tools/socgen -- which is always correct for a
// checkout that has this package at tools/socgen/elaborate.
var root string

// SetRoot records the repository root so CPUSynthConfig can locate
// components/cpu/variants.toml. Call it once, before the first
// CPUSynthConfig/CPUSynthConfigFPGAOpt call, from cmd/socgen's -root flag.
func SetRoot(r string) {
	root = r
	variantsOnce = sync.Once{}
	variantsCache = nil
	variantsErr = nil
}

func defaultRoot() string {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return "."
	}
	// file is .../tools/socgen/elaborate/variants.go; the repo root is three
	// directories up.
	return filepath.Join(filepath.Dir(file), "..", "..", "..")
}

var (
	variantsOnce  sync.Once
	variantsCache map[string]cpuVariant
	variantsErr   error
)

// loadVariants parses components/cpu/variants.toml (resolved from root, or
// defaultRoot() if SetRoot was never called) and caches the result. It
// returns a clear error -- never a silently-empty table -- if the file is
// absent, so a missing/uninitialised submodule fails loudly instead of
// reintroducing a hardcoded duplicate.
func loadVariants() (map[string]cpuVariant, error) {
	variantsOnce.Do(func() {
		r := root
		if r == "" {
			r = defaultRoot()
		}
		path := filepath.Join(r, "components", "cpu", "variants.toml")
		var parsed map[string]cpuVariant
		if _, err := toml.DecodeFile(path, &parsed); err != nil {
			variantsErr = fmt.Errorf("read CPU variant table %s: %w", path, err)
			return
		}
		variantsCache = parsed
	})
	return variantsCache, variantsErr
}

// variantFor returns the parsed cpuVariant for model (e.g. "j4"), erroring
// clearly if variants.toml is unreadable or does not define the model.
func variantFor(model string) (cpuVariant, error) {
	variants, err := loadVariants()
	if err != nil {
		return cpuVariant{}, err
	}
	v, ok := variants[model]
	if !ok {
		return cpuVariant{}, fmt.Errorf("model %q not defined in components/cpu/variants.toml", model)
	}
	return v, nil
}
