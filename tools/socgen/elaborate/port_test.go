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
	ports := buildPorts("dev0", e, spec, env, merge, false, nil)
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

	dev := buildPorts("aic0", e, noSpec, nil, noMerge, false, nil) // device default: prefixed
	if g := gsOf(dev, "cpu0_event_o"); g != "aic0_cpu0_event_o" {
		t.Errorf("device default = %q, want aic0_cpu0_event_o", g)
	}
	top := buildPorts("cpus", e, noSpec, nil, noMerge, true, nil) // top/padring default: bare
	if g := gsOf(top, "cpu0_event_o"); g != "cpu0_event_o" {
		t.Errorf("top default = %q, want bare cpu0_event_o", g)
	}
	spec := map[string]design.Value{"cpu0_event_o": {Kind: design.KindExpr, Text: "wired_sig"}}
	tw := buildPorts("cpus", e, spec, nil, noMerge, true, nil) // explicit mapping still wins
	if g := gsOf(tw, "cpu0_event_o"); g != "wired_sig" {
		t.Errorf("explicit mapping = %q, want wired_sig", g)
	}
}

func TestClkRstHeuristic(t *testing.T) {
	noMerge := map[string]string{}
	e := ent(iport("clk", "in"), iport("rst", "in"), iport("data", "out"))
	ps := buildPorts("aic0", e, map[string]design.Value{}, nil, noMerge, false, nil)
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
	ps := buildPorts("d", e, map[string]design.Value{}, nil, noMerge, false, nil)
	if gsOf(ps, "clk") == "clk_sys" || gsOf(ps, "clk_bus") == "clk_sys" {
		t.Errorf("ambiguous clk ports must not map to clk_sys: %q / %q", gsOf(ps, "clk"), gsOf(ps, "clk_bus"))
	}
	// explicit mapping wins over heuristic
	spec := map[string]design.Value{"clk": {Kind: design.KindExpr, Text: "myclk"}}
	ps2 := buildPorts("d", ent(iport("clk", "in")), spec, nil, noMerge, false, nil)
	if g := gsOf(ps2, "clk"); g != "myclk" {
		t.Errorf("explicit clk mapping = %q, want myclk", g)
	}
	// skip when target already used by another NON-EXPLICIT port's global-signal
	e3 := ent(iport("clk", "in"), iport("x", "in"))
	spec3 := map[string]design.Value{"x": {Kind: design.KindExpr, Text: "clk_sys"}}
	ps3 := buildPorts("d", e3, spec3, nil, noMerge, false, nil)
	// x is explicit and holds clk_sys; clk is still allowed to map to clk_sys
	if g := gsOf(ps3, "clk"); g != "clk_sys" {
		t.Errorf("clk should map to clk_sys even when an explicit port holds clk_sys, got %q", g)
	}
}

func TestBuildPortsSocPortNames(t *testing.T) {
	noSpec := map[string]design.Value{}
	noMerge := map[string]string{}

	// soc_port_global_name -> bare global signal (no device prefix), even for a device.
	eg := &iface.Entity{Name: "pio", Ports: []*iface.Port{
		{Name: "p_o", Dir: "out", Type: iface.TypeRef{Mark: "std_logic"}, GlobalName: "po"},
	}}
	pg := buildPorts("gpio0", eg, noSpec, nil, noMerge, false, nil)
	if g := gsOf(pg, "p_o"); g != "po" {
		t.Errorf("p_o GlobalName apply = %q, want bare po", g)
	}

	// soc_port_local_name -> prefix suffix (devName_<local>) for a device.
	el := &iface.Entity{Name: "spi", Ports: []*iface.Port{
		{Name: "spi_clk", Dir: "out", Type: iface.TypeRef{Mark: "std_logic"}, LocalName: "clk"},
	}}
	pl := buildPorts("flash", el, noSpec, nil, noMerge, false, nil)
	if g := gsOf(pl, "spi_clk"); g != "flash_clk" {
		t.Errorf("spi_clk LocalName apply = %q, want flash_clk", g)
	}

	// LocalName under bareDefault (top/padring): the suffix is used bare (no prefix).
	plb := buildPorts("flash", el, noSpec, nil, noMerge, true, nil)
	if g := gsOf(plb, "spi_clk"); g != "clk" {
		t.Errorf("spi_clk LocalName bare = %q, want clk", g)
	}

	// explicit design mapping still wins over GlobalName.
	ee := &iface.Entity{Name: "pio", Ports: []*iface.Port{
		{Name: "p_o", Dir: "out", Type: iface.TypeRef{Mark: "std_logic"}, GlobalName: "po"},
	}}
	spec := map[string]design.Value{"p_o": {Kind: design.KindExpr, Text: "explicit_sig"}}
	pe := buildPorts("gpio0", ee, spec, nil, noMerge, false, nil)
	if g := gsOf(pe, "p_o"); g != "explicit_sig" {
		t.Errorf("explicit mapping = %q, want explicit_sig (wins over GlobalName)", g)
	}
}

func TestBuildPortsSocPortIRQ(t *testing.T) {
	noSpec := map[string]design.Value{}
	noMerge := map[string]string{}

	// A soc_port_irq output with no device spec entry becomes KindIRQ
	// (wired by the IRQ model later), while a normal port stays KindSignal.
	e := &iface.Entity{Name: "gpio", Ports: []*iface.Port{
		{Name: "irq", Dir: "out", Type: iface.TypeRef{Mark: "std_logic"}, IRQ: true},
		{Name: "p_o", Dir: "out", Type: iface.TypeRef{Mark: "std_logic"}},
	}}
	ports := buildPorts("gpio0", e, noSpec, nil, noMerge, false, nil)
	byName := map[string]*ResolvedPort{}
	for _, p := range ports {
		byName[p.Name] = p
	}
	if byName["irq"].Kind != KindIRQ {
		t.Errorf("irq kind = %v, want KindIRQ", byName["irq"].Kind)
	}
	if byName["p_o"].Kind != KindSignal {
		t.Errorf("p_o kind = %v, want KindSignal", byName["p_o"].Kind)
	}

	// An explicit design mapping on the irq port wins (stays its explicit signal).
	spec := map[string]design.Value{"irq": {Kind: design.KindExpr, Text: "wired_irq"}}
	pe := buildPorts("gpio0", e, spec, nil, noMerge, false, nil)
	if g := gsOf(pe, "irq"); g != "wired_irq" {
		t.Errorf("explicit irq mapping = %q, want wired_irq", g)
	}
}

func TestClkRstHeuristicCaseAndExplicit(t *testing.T) {
	// Uppercase RST on a non-explicit port maps to reset (case-insensitive).
	e := ent(iport("RST", "in"), iport("db", "in"))
	ps := buildPorts("d", e, map[string]design.Value{}, nil, nil, true, nil)
	var rst *ResolvedPort
	for _, p := range ps {
		if p.Name == "RST" {
			rst = p
		}
	}
	if rst == nil || rst.GlobalSignal != "reset" {
		t.Errorf("RST should map to reset, got %+v", rst)
	}
	// An explicit port already holding clk_sys must NOT block clk->clk_sys.
	e2 := ent(iport("clk", "in"), iport("clk_ddr", "in"))
	spec := map[string]design.Value{"clk_ddr": {Kind: design.KindExpr, Text: "clk_sys"}}
	ps2 := buildPorts("d", e2, spec, nil, nil, true, nil)
	var clk *ResolvedPort
	for _, p := range ps2 {
		if p.Name == "clk" {
			clk = p
		}
	}
	if clk == nil || clk.GlobalSignal != "clk_sys" {
		t.Errorf("clk should map to clk_sys despite explicit clk_ddr, got %+v", clk)
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

func TestBuildPortsConstantValue(t *testing.T) {
	lib := buildLib(t, `package p is constant NULL_X : integer := 0; end package;`,
		`entity e is port (cin : in integer); end entity;`)
	e, ok := lib.Entity("e")
	if !ok {
		t.Fatal("entity e not found")
	}
	spec := map[string]design.Value{"cin": {Kind: design.KindExpr, Text: "NULL_X"}}
	ps := buildPorts("d", e, spec, nil, nil, true, lib)
	if len(ps) != 1 || ps[0].Kind != KindValue || ps[0].GlobalSignal != "" ||
		ps[0].Value == nil || ps[0].Value.Text != "NULL_X" {
		t.Errorf("constant-valued port should be KindValue carrying NULL_X, no GlobalSignal: %+v", ps[0])
	}
}
