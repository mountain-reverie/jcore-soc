package elaborate

import (
	"errors"
	"sort"
	"testing"
)

// csig builds a net-list signal with the given (contextKind, dir) ports.
// (Named csig because netlist_validate_test.go already defines a different `sig`.)
func csig(name string, ports ...[2]string) *Signal {
	s := &Signal{Name: name, Type: &ResolvedType{Mark: "std_logic"}}
	for _, p := range ports {
		s.Ports = append(s.Ports, &SignalPortRef{Context: Context{Kind: p[0]}, Dir: p[1]})
	}
	return s
}

func TestTypeCombinations(t *testing.T) {
	keys := map[string]bool{}
	for _, set := range typeCombinations([]string{"a", "b"}, []string{"c"}) {
		keys[setKey(set)] = true
	}
	if len(keys) != 2 || !keys["a,c"] || !keys["b,c"] {
		t.Errorf("typeCombinations = %v", keys)
	}
	// "" picks are dropped: {a,b} and {a}
	k2 := map[string]bool{}
	for _, set := range typeCombinations([]string{"a"}, []string{"b", ""}) {
		k2[setKey(set)] = true
	}
	if !k2["a,b"] || !k2["a"] {
		t.Errorf("typeCombinations with empty = %v", k2)
	}
}

func TestSrcContext(t *testing.T) {
	c, err := srcContext(csig("s", [2]string{"pin", "out"}, [2]string{"top", "in"}))
	if err != nil || c != "padring" { // pin normalizes to padring
		t.Errorf("srcContext = %q, %v (want padring)", c, err)
	}
	_, err = srcContext(csig("s", [2]string{"device", "out"}, [2]string{"top", "out"}))
	if err == nil || !errors.Is(err, ErrMultiContextDriver) {
		t.Errorf("want ErrMultiContextDriver, got %v", err)
	}
	_, err = srcContext(csig("s", [2]string{"top", "in"}))
	if err == nil || !errors.Is(err, ErrMultiContextDriver) {
		t.Errorf("want ErrMultiContextDriver for zero out-ports, got %v", err)
	}
}

func TestInOutSignals(t *testing.T) {
	in := csig("a", [2]string{"device", "in"}, [2]string{"device", "out"})
	out := csig("b", [2]string{"device", "out"}, [2]string{"top", "in"})
	got := inOutSignals([]*Signal{in, out}, "device")
	if len(got) != 1 || got[0].Name != "a" {
		t.Errorf("inOutSignals = %v", names(got))
	}
}

func TestContextSetsAndFilter(t *testing.T) {
	sigs := map[string]*Signal{
		"x": csig("x", [2]string{"device", "out"}, [2]string{"top", "in"}),    // {device,top}
		"y": csig("y", [2]string{"device", "out"}, [2]string{"device", "in"}), // {device}
	}
	cs := contextSets(sigs)
	got := filterByContext(cs, []string{"top"}, []string{"device"})
	if len(got) != 1 || got[0].Name != "x" {
		t.Errorf("filterByContext = %v", names(got))
	}
}

func TestCategorize(t *testing.T) {
	res := &Resolution{
		Signals: map[string]*Signal{
			"d2t": csig("d2t", [2]string{"device", "out"}, [2]string{"top", "in"}),  // top<->devices, src device
			"p2d": csig("p2d", [2]string{"pin", "out"}, [2]string{"device", "in"}),   // padring<->devices, src pin->padring
			"p2t": csig("p2t", [2]string{"pin", "out"}, [2]string{"top", "in"}),      // padring<->top, src pin->padring
			"dint": csig("dint", [2]string{"device", "out"}, [2]string{"device", "in"}), // device-internal, in+out
		},
	}
	sl, err := categorize(res)
	if err != nil {
		t.Fatalf("categorize: %v", err)
	}
	td := portMap(sl.TopDevices)
	if td["d2t"] != "out" || td["p2d"] != "in" {
		t.Errorf("TopDevices = %v (want d2t:out, p2d:in)", td)
	}
	if _, ok := td["dint"]; ok {
		t.Errorf("dint (device-internal) must NOT be a devices port: %v", td)
	}
	if pt := portMap(sl.PadringTop); pt["p2t"] != "in" {
		t.Errorf("PadringTop = %v (want p2t:in)", pt)
	}
	if !contains(sl.Devices, "dint") {
		t.Errorf("Devices internal = %v (want dint)", sl.Devices)
	}
	// dint is in+out within device but NOT a top-devices boundary signal, so it is
	// device-internal only and NOT in DevicesExtra (which is computed from the
	// top-devices set). Confirm faithful behavior:
	if _, ok := sl.DevicesExtra["dint"]; ok {
		t.Errorf("DevicesExtra should not contain a purely-internal signal: %v", sl.DevicesExtra)
	}
}

func portMap(ps []PortLoc) map[string]string {
	m := map[string]string{}
	for _, p := range ps {
		m[p.Name] = p.Dir
	}
	return m
}
func contains(ss []string, x string) bool {
	for _, s := range ss {
		if s == x {
			return true
		}
	}
	return false
}

func names(ss []*Signal) []string {
	out := make([]string, 0, len(ss))
	for _, s := range ss {
		out = append(out, s.Name)
	}
	sort.Strings(out)
	return out
}
