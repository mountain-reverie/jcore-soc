package elaborate

import (
	"errors"
	"testing"

	"github.com/j-core/jcore-soc/tools/socgen/design"
	"github.com/j-core/jcore-soc/tools/socgen/internal/errutil"
)

func TestChooseArchSingle(t *testing.T) {
	lib := buildLib(t,
		`entity e is port (clk : in std_logic); end entity;`,
		`architecture rtl of e is begin end architecture;`)
	ent, arch, cfg, hardErr, err := chooseArch(`class "e"`, "e", "", "", lib)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if ent == nil || arch != "rtl" || cfg != nil || hardErr {
		t.Fatalf("got ent=%v arch=%q cfg=%v hardErr=%v", ent, arch, cfg, hardErr)
	}
}

func TestChooseArchEntityNotFound(t *testing.T) {
	lib := buildLib(t, `entity e is port (clk : in std_logic); end entity;`,
		`architecture rtl of e is begin end architecture;`)
	ent, _, _, hardErr, err := chooseArch(`class "ghost"`, "ghost", "", "", lib)
	if ent != nil || !hardErr || !errors.Is(err, ErrEntityNotFound) {
		t.Fatalf("expected hard error for missing entity: ent=%v hardErr=%v err=%v", ent, hardErr, err)
	}
}

func TestChooseArchAmbiguousIsSoft(t *testing.T) {
	// two architectures, none named -> error but hardErr=false (faithful: falls through)
	lib := buildLib(t,
		`entity e is port (clk : in std_logic); end entity;`,
		`architecture a1 of e is begin end architecture;`,
		`architecture a2 of e is begin end architecture;`)
	ent, arch, _, hardErr, err := chooseArch(`class "e"`, "e", "", "", lib)
	if ent == nil || arch != "" || hardErr || !errors.Is(err, ErrAmbiguousArch) {
		t.Fatalf("expected soft ambiguity: ent=%v arch=%q hardErr=%v err=%v", ent, arch, hardErr, err)
	}
	_ = design.KindExpr // keep design import used across the file
}

func TestChooseArchCfgAndArchAgree(t *testing.T) {
	// both configuration and architecture given and AGREEING -> resolves via config
	lib := buildLib(t,
		`entity e is end entity;`,
		`architecture rtl of e is begin end architecture;`,
		`architecture other of e is begin end architecture;`,
		`configuration ecfg of e is for rtl end for; end configuration;`)
	ent, arch, cfg, hardErr, err := chooseArch(`class "e"`, "e", "rtl", "ecfg", lib)
	if err != nil || hardErr {
		t.Fatalf("agreeing arch+config should resolve cleanly: hardErr=%v err=%v", hardErr, err)
	}
	if ent == nil || arch != "rtl" || cfg == nil || cfg.Name != "ecfg" {
		t.Fatalf("got ent=%v arch=%q cfg=%v", ent, arch, cfg)
	}
}

func TestResolveEntityExplicitEntity(t *testing.T) {
	lib := buildLib(t,
		`entity pll is port (clk_i : in std_logic; clk_o : out std_logic); end entity;`,
		`architecture rtl of pll is begin end architecture;`)
	te := &design.TopEntity{
		Entity: "pll",
		Ports:  map[string]design.Value{"clk_i": {Kind: design.KindExpr, Text: "ref"}, "clk_o": {Kind: design.KindExpr, Text: "sys"}},
	}
	re, err := resolveEntity("padring", "mypll", te, lib, map[string]string{})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if re.Name != "mypll" || re.Entity == nil || re.ArchName != "rtl" {
		t.Fatalf("got %+v", re)
	}
	if len(re.Ports) != 2 {
		t.Fatalf("expected 2 ports, got %d", len(re.Ports))
	}
}

func TestResolveEntityNameDefaultsToKey(t *testing.T) {
	// no Entity field -> the map key is used as the entity name
	lib := buildLib(t,
		`entity fpga_reboot is port (clk : in std_logic); end entity;`,
		`architecture s6 of fpga_reboot is begin end architecture;`)
	te := &design.TopEntity{Architecture: "s6"}
	re, err := resolveEntity("top", "fpga_reboot", te, lib, map[string]string{})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if re.Entity == nil || re.ArchName != "s6" {
		t.Fatalf("entity-name-from-key or architecture failed: %+v", re)
	}
}

func TestResolveEntityUnknownEntity(t *testing.T) {
	lib := buildLib(t, `entity e is port (clk : in std_logic); end entity;`,
		`architecture rtl of e is begin end architecture;`)
	re, err := resolveEntity("top", "ghost", &design.TopEntity{}, lib, map[string]string{})
	if re.Entity != nil || !errors.Is(err, ErrEntityNotFound) {
		t.Fatalf("expected one error and nil entity: re=%+v err=%v", re, err)
	}
}

func TestResolveEntityGenerics(t *testing.T) {
	te := &design.TopEntity{
		Entity:   "e",
		Generics: map[string]design.Value{"A": {Kind: design.KindInt, Int: 1}, "B": {Kind: design.KindInt, Int: 2}},
	}
	lib := buildLib(t, `entity e is generic (A : integer; B : integer); port (clk : in std_logic); end entity;`)
	re, _ := resolveEntity("top", "e", te, lib, nil)
	if re.Generics == nil || len(re.Generics) != 2 {
		t.Fatalf("re.Generics = %v, want 2 entries", re.Generics)
	}
	if re.Generics["A"].Int != 1 || re.Generics["B"].Int != 2 {
		t.Errorf("re.Generics values wrong: %+v", re.Generics)
	}
	te2 := &design.TopEntity{Entity: "e"}
	re2, _ := resolveEntity("top", "e", te2, lib, nil)
	if len(re2.Generics) != 0 {
		t.Errorf("re2.Generics = %v, want empty", re2.Generics)
	}
}

func TestResolveEntitiesSortedAndAccumulates(t *testing.T) {
	lib := buildLib(t,
		`entity a is port (clk : in std_logic); end entity;`,
		`architecture rtl of a is begin end architecture;`,
		`entity b is port (clk : in std_logic); end entity;`,
		`architecture rtl of b is begin end architecture;`)
	// empty map -> empty result, no errors
	out, err := resolveEntities("top", nil, lib, map[string]string{})
	if len(out) != 0 || err != nil {
		t.Fatalf("empty map: out=%v err=%v", out, err)
	}
	// two entries (one resolvable, one unknown entity) -> both keyed, one error
	ents := map[string]*design.TopEntity{
		"x": {Entity: "a"},
		"y": {Entity: "ghost"},
	}
	out, err = resolveEntities("top", ents, lib, map[string]string{})
	if len(out) != 2 || out["x"] == nil || out["y"] == nil {
		t.Fatalf("expected both keys resolved: %v", out)
	}
	if out["x"].Entity == nil {
		t.Errorf("x should bind entity a")
	}
	if out["y"].Entity != nil {
		t.Errorf("y should have nil entity (ghost)")
	}
	if n := len(errutil.Errors(err)); n != 1 {
		t.Errorf("expected exactly one error (ghost), got %d: %v", n, err)
	}
}
