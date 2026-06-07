package elaborate

import (
	"testing"

	"github.com/j-core/jcore-soc/tools/socgen/design"
)

func TestChooseArchSingle(t *testing.T) {
	lib := buildLib(t,
		`entity e is port (clk : in std_logic); end entity;`,
		`architecture rtl of e is begin end architecture;`)
	ent, arch, cfg, hardErr, errs := chooseArch(`class "e"`, "e", "", "", lib, nil)
	if len(errs) != 0 {
		t.Fatalf("errs: %v", errs)
	}
	if ent == nil || arch != "rtl" || cfg != nil || hardErr {
		t.Fatalf("got ent=%v arch=%q cfg=%v hardErr=%v", ent, arch, cfg, hardErr)
	}
}

func TestChooseArchEntityNotFound(t *testing.T) {
	lib := buildLib(t, `entity e is port (clk : in std_logic); end entity;`,
		`architecture rtl of e is begin end architecture;`)
	ent, _, _, hardErr, errs := chooseArch(`class "ghost"`, "ghost", "", "", lib, nil)
	if ent != nil || !hardErr || len(errs) != 1 {
		t.Fatalf("expected hard error for missing entity: ent=%v hardErr=%v errs=%v", ent, hardErr, errs)
	}
}

func TestChooseArchAmbiguousIsSoft(t *testing.T) {
	// two architectures, none named -> error but hardErr=false (faithful: falls through)
	lib := buildLib(t,
		`entity e is port (clk : in std_logic); end entity;`,
		`architecture a1 of e is begin end architecture;`,
		`architecture a2 of e is begin end architecture;`)
	ent, arch, _, hardErr, errs := chooseArch(`class "e"`, "e", "", "", lib, nil)
	if ent == nil || arch != "" || hardErr || len(errs) != 1 {
		t.Fatalf("expected soft ambiguity: ent=%v arch=%q hardErr=%v errs=%v", ent, arch, hardErr, errs)
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
	ent, arch, cfg, hardErr, errs := chooseArch(`class "e"`, "e", "rtl", "ecfg", lib, nil)
	if len(errs) != 0 || hardErr {
		t.Fatalf("agreeing arch+config should resolve cleanly: hardErr=%v errs=%v", hardErr, errs)
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
	re, errs := resolveEntity("padring", "mypll", te, lib, map[string]string{}, nil)
	if len(errs) != 0 {
		t.Fatalf("errs: %v", errs)
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
	re, errs := resolveEntity("top", "fpga_reboot", te, lib, map[string]string{}, nil)
	if len(errs) != 0 {
		t.Fatalf("errs: %v", errs)
	}
	if re.Entity == nil || re.ArchName != "s6" {
		t.Fatalf("entity-name-from-key or architecture failed: %+v", re)
	}
}

func TestResolveEntityUnknownEntity(t *testing.T) {
	lib := buildLib(t, `entity e is port (clk : in std_logic); end entity;`,
		`architecture rtl of e is begin end architecture;`)
	re, errs := resolveEntity("top", "ghost", &design.TopEntity{}, lib, map[string]string{}, nil)
	if re.Entity != nil || len(errs) != 1 {
		t.Fatalf("expected one error and nil entity: re=%+v errs=%v", re, errs)
	}
}

func TestResolveEntitiesSortedAndAccumulates(t *testing.T) {
	lib := buildLib(t,
		`entity a is port (clk : in std_logic); end entity;`,
		`architecture rtl of a is begin end architecture;`,
		`entity b is port (clk : in std_logic); end entity;`,
		`architecture rtl of b is begin end architecture;`)
	// empty map -> empty result, no errors
	out, errs := resolveEntities("top", nil, lib, map[string]string{}, nil)
	if len(out) != 0 || len(errs) != 0 {
		t.Fatalf("empty map: out=%v errs=%v", out, errs)
	}
	// two entries (one resolvable, one unknown entity) -> both keyed, one error
	ents := map[string]*design.TopEntity{
		"x": {Entity: "a"},
		"y": {Entity: "ghost"},
	}
	out, errs = resolveEntities("top", ents, lib, map[string]string{}, nil)
	if len(out) != 2 || out["x"] == nil || out["y"] == nil {
		t.Fatalf("expected both keys resolved: %v", out)
	}
	if out["x"].Entity == nil {
		t.Errorf("x should bind entity a")
	}
	if out["y"].Entity != nil {
		t.Errorf("y should have nil entity (ghost)")
	}
	if len(errs) != 1 {
		t.Errorf("expected exactly one error (ghost), got %v", errs)
	}
}
