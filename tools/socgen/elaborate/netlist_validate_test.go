package elaborate

import (
	"strings"
	"testing"
)

func sig(name string, ports ...*SignalPortRef) *Signal {
	var t *ResolvedType
	if len(ports) > 0 {
		t = ports[0].Type
	}
	return &Signal{Name: name, Type: t, Ports: ports}
}
func pr(id, port, dir, typ string) *SignalPortRef {
	return &SignalPortRef{Context: Context{Kind: "device", ID: id}, PortName: port, Dir: dir, Type: &ResolvedType{Mark: typ}}
}

func TestValidateSignals(t *testing.T) {
	// multiple drivers
	multi := map[string]*Signal{"x": sig("x", pr("a", "o1", "out", "std_logic"), pr("b", "o2", "out", "std_logic"))}
	if errs := validateSignals(multi, nil); len(errs) == 0 || !strings.Contains(errs[0].Error(), "multiple") {
		t.Errorf("want multiple-driver error, got %v", errs)
	}
	// undriven
	und := map[string]*Signal{"y": sig("y", pr("a", "i1", "in", "std_logic"))}
	if errs := validateSignals(und, nil); len(errs) == 0 || !strings.Contains(errs[0].Error(), "nothing drives") {
		t.Errorf("want undriven error, got %v", errs)
	}
	// type mismatch
	mis := map[string]*Signal{"z": sig("z", pr("a", "o", "out", "std_logic"), pr("b", "i", "in", "std_logic_vector(7 downto 0)"))}
	if errs := validateSignals(mis, nil); len(errs) == 0 || !strings.Contains(errs[0].Error(), "type mismatch") {
		t.Errorf("want type-mismatch error, got %v", errs)
	}
	// clean: one out, one in, same type
	clean := map[string]*Signal{"w": sig("w", pr("a", "o", "out", "std_logic"), pr("b", "i", "in", "std_logic"))}
	if errs := validateSignals(clean, nil); len(errs) != 0 {
		t.Errorf("clean signal should pass: %v", errs)
	}
}
