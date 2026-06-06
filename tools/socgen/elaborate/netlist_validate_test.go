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

func TestValidateDifferentialException(t *testing.T) {
	// two pin drivers forming a pos/neg pair -> allowed
	sigs := map[string]*Signal{"ddr_clk": {Name: "ddr_clk", Type: &ResolvedType{Mark: "std_logic"}, Ports: []*SignalPortRef{
		{Context: Context{Kind: "pin", ID: "ck"}, Dir: "out", Type: &ResolvedType{Mark: "std_logic"}, Diff: "pos"},
		{Context: Context{Kind: "pin", ID: "ck_n"}, Dir: "out", Type: &ResolvedType{Mark: "std_logic"}, Diff: "neg"},
	}}}
	if errs := validateSignals(sigs, nil); len(errs) != 0 {
		t.Errorf("differential pair should be allowed: %v", errs)
	}
}

func TestValidateMultiElementException(t *testing.T) {
	// multiple pin drivers on distinct elements -> allowed
	sigs := map[string]*Signal{"dr_data_i": {Name: "dr_data_i", Type: &ResolvedType{Mark: "std_logic"}, Ports: []*SignalPortRef{
		{Context: Context{Kind: "pin", ID: "dq0"}, Dir: "out", Type: &ResolvedType{Mark: "std_logic"}, Element: "dr_data_i.dqi(0)"},
		{Context: Context{Kind: "pin", ID: "dq1"}, Dir: "out", Type: &ResolvedType{Mark: "std_logic"}, Element: "dr_data_i.dqi(1)"},
	}}}
	if errs := validateSignals(sigs, nil); len(errs) != 0 {
		t.Errorf("distinct-element pins should be allowed: %v", errs)
	}
}

func TestValidateMultiDriverStillErrors(t *testing.T) {
	// two whole-signal device drivers -> still an error
	sigs := map[string]*Signal{"s": {Name: "s", Type: &ResolvedType{Mark: "std_logic"}, Ports: []*SignalPortRef{
		{Context: Context{Kind: "device", ID: "a"}, Dir: "out", Type: &ResolvedType{Mark: "std_logic"}},
		{Context: Context{Kind: "device", ID: "b"}, Dir: "out", Type: &ResolvedType{Mark: "std_logic"}},
	}}}
	errs := validateSignals(sigs, nil)
	found := false
	for _, e := range errs {
		if strings.Contains(e.Error(), "driven by multiple ports") {
			found = true
		}
	}
	if !found {
		t.Errorf("two device drivers should error; got %v", errs)
	}
}

func TestValidateSameElementTwiceErrors(t *testing.T) {
	// two pins driving the SAME element -> still an error (not distinct)
	sigs := map[string]*Signal{"bus": {Name: "bus", Type: &ResolvedType{Mark: "std_logic"}, Ports: []*SignalPortRef{
		{Context: Context{Kind: "pin", ID: "a"}, Dir: "out", Type: &ResolvedType{Mark: "std_logic"}, Element: "bus.x(0)"},
		{Context: Context{Kind: "pin", ID: "b"}, Dir: "out", Type: &ResolvedType{Mark: "std_logic"}, Element: "bus.x(0)"},
	}}}
	errs := validateSignals(sigs, nil)
	found := false
	for _, e := range errs {
		if strings.Contains(e.Error(), "driven by multiple ports") {
			found = true
		}
	}
	if !found {
		t.Errorf("two pins on the same element should error; got %v", errs)
	}
}
