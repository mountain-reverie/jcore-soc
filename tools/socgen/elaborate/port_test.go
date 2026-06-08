package elaborate

import (
	"testing"

	"github.com/j-core/jcore-soc/tools/socgen/design"
	"github.com/j-core/jcore-soc/tools/socgen/iface"
)

func TestBuildPorts(t *testing.T) {
	ent := buildLib(t,
		`entity e is generic (num_cs : integer); port (en : in std_logic; cs : out std_logic_vector(num_cs-1 downto 0); irq : out std_logic; lit : in std_logic; open_port : in std_logic); end entity;`,
	)
	e, _ := ent.Entity("e")
	env := map[string]int64{"num_cs": 2}
	// merge renames the explicit signal "flash_cs" to "flash_cs_merged"
	merge := reverseMerge(map[string][]string{
		"merged_en":       {"dev0_en"},
		"flash_cs_merged": {"flash_cs"},
	})
	spec := map[string]design.Value{
		"cs":        {Kind: design.KindExpr, Text: "flash_cs"}, // explicit signal -> renamed by merge
		"irq":       {Kind: design.KindMap, Map: map[string]any{"irq?": true}},
		"lit":       {Kind: design.KindInt, Int: 0}, // constant value
		"open_port": {Kind: design.KindMap, Map: map[string]any{"open?": true}}, // KindDeferred
		// en unspecified -> autogen dev0_en -> merged_en
	}
	ports := buildPorts("dev0", e, spec, env, merge, false)
	byName := map[string]*ResolvedPort{}
	for _, p := range ports {
		byName[p.Name] = p
	}
	if byName["en"].GlobalSignal != "merged_en" { // autogen + merge
		t.Errorf("en signal = %q", byName["en"].GlobalSignal)
	}
	if byName["cs"].GlobalSignal != "flash_cs_merged" { // explicit signal renamed by merge
		t.Errorf("cs GlobalSignal = %q, want flash_cs_merged", byName["cs"].GlobalSignal)
	}
	if byName["cs"].Type.String() != "std_logic_vector(1 downto 0)" {
		t.Errorf("cs type = %s", byName["cs"].Type.String())
	}
	if byName["irq"].Kind != KindIRQ {
		t.Errorf("irq kind = %v", byName["irq"].Kind)
	}
	if byName["lit"].Kind != KindValue || byName["lit"].Value.Int != 0 {
		t.Errorf("lit = %+v", byName["lit"])
	}
	// KindDeferred: open? map -> KindDeferred and GlobalSignal == ""
	if byName["open_port"].Kind != KindDeferred {
		t.Errorf("open_port kind = %v, want KindDeferred", byName["open_port"].Kind)
	}
	if byName["open_port"].GlobalSignal != "" {
		t.Errorf("open_port GlobalSignal = %q, want empty (deferred)", byName["open_port"].GlobalSignal)
	}
}

func ent(ports ...*iface.Port) *iface.Entity { return &iface.Entity{Name: "e", Ports: ports} }
func iport(name, dir string) *iface.Port {
	return &iface.Port{Name: name, Dir: dir, Type: iface.TypeRef{Mark: "std_logic"}}
}
func gsOf(ports []*ResolvedPort, name string) string {
	for _, p := range ports {
		if p.Name == name {
			return p.GlobalSignal
		}
	}
	return "<absent>"
}

func TestBuildPortsBareVsPrefixed(t *testing.T) {
	e := ent(iport("cpu0_event_o", "out"))
	noSpec := map[string]design.Value{}
	noMerge := map[string]string{}

	dev := buildPorts("aic0", e, noSpec, nil, noMerge, false) // device default: prefixed
	if g := gsOf(dev, "cpu0_event_o"); g != "aic0_cpu0_event_o" {
		t.Errorf("device default = %q, want aic0_cpu0_event_o", g)
	}
	top := buildPorts("cpus", e, noSpec, nil, noMerge, true) // top/padring default: bare
	if g := gsOf(top, "cpu0_event_o"); g != "cpu0_event_o" {
		t.Errorf("top default = %q, want bare cpu0_event_o", g)
	}
	spec := map[string]design.Value{"cpu0_event_o": {Kind: design.KindExpr, Text: "wired_sig"}}
	tw := buildPorts("cpus", e, spec, nil, noMerge, true) // explicit mapping still wins
	if g := gsOf(tw, "cpu0_event_o"); g != "wired_sig" {
		t.Errorf("explicit mapping = %q, want wired_sig", g)
	}
}

func TestClkRstHeuristic(t *testing.T) {
	noMerge := map[string]string{}
	e := ent(iport("clk", "in"), iport("rst", "in"), iport("data", "out"))
	ps := buildPorts("aic0", e, map[string]design.Value{}, nil, noMerge, false)
	if g := gsOf(ps, "clk"); g != "clk_sys" {
		t.Errorf("clk -> %q, want clk_sys", g)
	}
	if g := gsOf(ps, "rst"); g != "reset" {
		t.Errorf("rst -> %q, want reset", g)
	}
	if g := gsOf(ps, "data"); g != "aic0_data" {
		t.Errorf("data -> %q, want aic0_data (unchanged)", g)
	}
}

func TestClkRstHeuristicSkips(t *testing.T) {
	noMerge := map[string]string{}
	// ambiguous: two clk-ish ports -> skip both
	e := ent(iport("clk", "in"), iport("clk_bus", "in"))
	ps := buildPorts("d", e, map[string]design.Value{}, nil, noMerge, false)
	if gsOf(ps, "clk") == "clk_sys" || gsOf(ps, "clk_bus") == "clk_sys" {
		t.Errorf("ambiguous clk ports must not map to clk_sys: %q / %q", gsOf(ps, "clk"), gsOf(ps, "clk_bus"))
	}
	// explicit mapping wins over heuristic
	spec := map[string]design.Value{"clk": {Kind: design.KindExpr, Text: "myclk"}}
	ps2 := buildPorts("d", ent(iport("clk", "in")), spec, nil, noMerge, false)
	if g := gsOf(ps2, "clk"); g != "myclk" {
		t.Errorf("explicit clk mapping = %q, want myclk", g)
	}
	// skip when target already used by another port's global-signal
	e3 := ent(iport("clk", "in"), iport("x", "in"))
	spec3 := map[string]design.Value{"x": {Kind: design.KindExpr, Text: "clk_sys"}}
	ps3 := buildPorts("d", e3, spec3, nil, noMerge, false)
	if g := gsOf(ps3, "clk"); g == "clk_sys" {
		t.Errorf("clk must not map to clk_sys when already used by another port")
	}
}

func TestGenericEnv(t *testing.T) {
	lib := buildLib(t, `entity e is generic (w : integer := 8; n : integer); end entity;`)
	e, _ := lib.Entity("e")
	// device passes w:4, which should override the entity default of 8
	env := genericEnv(map[string]design.Value{
		"n": {Kind: design.KindInt, Int: 3},
		"w": {Kind: design.KindInt, Int: 4}, // conflicts with entity default w:=8; device wins
	}, e)
	if env["w"] != 4 { // device override wins over entity default
		t.Errorf("w = %d want 4 (device override wins)", env["w"])
	}
	if env["n"] != 3 { // device override
		t.Errorf("n = %d want 3", env["n"])
	}
}
