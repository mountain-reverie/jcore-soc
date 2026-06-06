package elaborate

import (
	"testing"

	"github.com/j-core/jcore-soc/tools/socgen/design"
)

func TestBuildPorts(t *testing.T) {
	ent := buildLib(t,
		`entity e is generic (num_cs : integer); port (clk : in std_logic; cs : out std_logic_vector(num_cs-1 downto 0); irq : out std_logic; lit : in std_logic); end entity;`,
	)
	e, _ := ent.Entity("e")
	env := map[string]int64{"num_cs": 2}
	merge := reverseMerge(map[string][]string{"merged_clk": {"dev0_clk"}})
	spec := map[string]design.Value{
		"cs":  {Kind: design.KindExpr, Text: "flash_cs"}, // explicit signal
		"irq": {Kind: design.KindMap, Map: map[string]any{"irq?": true}},
		"lit": {Kind: design.KindInt, Int: 0}, // constant value
		// clk unspecified -> autogen dev0_clk -> merged_clk
	}
	ports := buildPorts("dev0", e, spec, env, merge)
	byName := map[string]*ResolvedPort{}
	for _, p := range ports {
		byName[p.Name] = p
	}
	if byName["clk"].GlobalSignal != "merged_clk" { // autogen + merge
		t.Errorf("clk signal = %q", byName["clk"].GlobalSignal)
	}
	if byName["cs"].GlobalSignal != "flash_cs" || byName["cs"].Type.String() != "std_logic_vector(1 downto 0)" {
		t.Errorf("cs = %+v (%s)", byName["cs"], byName["cs"].Type.String())
	}
	if byName["irq"].Kind != KindIRQ {
		t.Errorf("irq kind = %v", byName["irq"].Kind)
	}
	if byName["lit"].Kind != KindValue || byName["lit"].Value.Int != 0 {
		t.Errorf("lit = %+v", byName["lit"])
	}
}

func TestGenericEnv(t *testing.T) {
	lib := buildLib(t, `entity e is generic (w : integer := 8; n : integer); end entity;`)
	e, _ := lib.Entity("e")
	env := genericEnv(map[string]design.Value{"n": {Kind: design.KindInt, Int: 3}}, e)
	if env["w"] != 8 { // entity default
		t.Errorf("w = %d want 8 (entity default)", env["w"])
	}
	if env["n"] != 3 { // device override
		t.Errorf("n = %d want 3", env["n"])
	}
}
