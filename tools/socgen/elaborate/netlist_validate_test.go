package elaborate

import (
	"errors"
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
	if err := validateSignals(multi); !errors.Is(err, ErrMultiDriver) {
		t.Errorf("want multiple-driver error, got %v", err)
	}
	// undriven
	und := map[string]*Signal{"y": sig("y", pr("a", "i1", "in", "std_logic"))}
	if err := validateSignals(und); !errors.Is(err, ErrUndrivenSignal) {
		t.Errorf("want undriven error, got %v", err)
	}
	// type mismatch
	mis := map[string]*Signal{"z": sig("z", pr("a", "o", "out", "std_logic"), pr("b", "i", "in", "std_logic_vector(7 downto 0)"))}
	if err := validateSignals(mis); !errors.Is(err, ErrTypeMismatch) {
		t.Errorf("want type-mismatch error, got %v", err)
	}
	// clean: one out, one in, same type
	clean := map[string]*Signal{"w": sig("w", pr("a", "o", "out", "std_logic"), pr("b", "i", "in", "std_logic"))}
	if err := validateSignals(clean); err != nil {
		t.Errorf("clean signal should pass: %v", err)
	}
}

func TestValidateDifferentialException(t *testing.T) {
	// two pin drivers forming a pos/neg pair -> allowed
	sigs := map[string]*Signal{"ddr_clk": {Name: "ddr_clk", Type: &ResolvedType{Mark: "std_logic"}, Ports: []*SignalPortRef{
		{Context: Context{Kind: "pin", ID: "ck"}, Dir: "out", Type: &ResolvedType{Mark: "std_logic"}, Diff: "pos"},
		{Context: Context{Kind: "pin", ID: "ck_n"}, Dir: "out", Type: &ResolvedType{Mark: "std_logic"}, Diff: "neg"},
	}}}
	if err := validateSignals(sigs); err != nil {
		t.Errorf("differential pair should be allowed: %v", err)
	}
}

func TestValidateMultiElementException(t *testing.T) {
	// multiple pin drivers on distinct elements -> allowed
	sigs := map[string]*Signal{"dr_data_i": {Name: "dr_data_i", Type: &ResolvedType{Mark: "std_logic"}, Ports: []*SignalPortRef{
		{Context: Context{Kind: "pin", ID: "dq0"}, Dir: "out", Type: &ResolvedType{Mark: "std_logic"}, Element: "dr_data_i.dqi(0)"},
		{Context: Context{Kind: "pin", ID: "dq1"}, Dir: "out", Type: &ResolvedType{Mark: "std_logic"}, Element: "dr_data_i.dqi(1)"},
	}}}
	if err := validateSignals(sigs); err != nil {
		t.Errorf("distinct-element pins should be allowed: %v", err)
	}
}

func TestValidateMultiDriverStillErrors(t *testing.T) {
	// two whole-signal device drivers -> still an error
	sigs := map[string]*Signal{"s": {Name: "s", Type: &ResolvedType{Mark: "std_logic"}, Ports: []*SignalPortRef{
		{Context: Context{Kind: "device", ID: "a"}, Dir: "out", Type: &ResolvedType{Mark: "std_logic"}},
		{Context: Context{Kind: "device", ID: "b"}, Dir: "out", Type: &ResolvedType{Mark: "std_logic"}},
	}}}
	if err := validateSignals(sigs); !errors.Is(err, ErrMultiDriver) {
		t.Errorf("two device drivers should error; got %v", err)
	}
}

func TestValidateSameElementTwiceErrors(t *testing.T) {
	// two pins driving the SAME element -> still an error (not distinct)
	sigs := map[string]*Signal{"bus": {Name: "bus", Type: &ResolvedType{Mark: "std_logic"}, Ports: []*SignalPortRef{
		{Context: Context{Kind: "pin", ID: "a"}, Dir: "out", Type: &ResolvedType{Mark: "std_logic"}, Element: "bus.x(0)"},
		{Context: Context{Kind: "pin", ID: "b"}, Dir: "out", Type: &ResolvedType{Mark: "std_logic"}, Element: "bus.x(0)"},
	}}}
	if err := validateSignals(sigs); !errors.Is(err, ErrMultiDriver) {
		t.Errorf("two pins on the same element should error; got %v", err)
	}
}

func TestValidateDifferentialBothPosErrors(t *testing.T) {
	// two pins both diff "pos" (not a real pos/neg pair) -> still an error
	sigs := map[string]*Signal{"s": {Name: "s", Type: &ResolvedType{Mark: "std_logic"}, Ports: []*SignalPortRef{
		{Context: Context{Kind: "pin", ID: "a"}, Dir: "out", Type: &ResolvedType{Mark: "std_logic"}, Diff: "pos"},
		{Context: Context{Kind: "pin", ID: "b"}, Dir: "out", Type: &ResolvedType{Mark: "std_logic"}, Diff: "pos"},
	}}}
	if err := validateSignals(sigs); !errors.Is(err, ErrMultiDriver) {
		t.Error("two pos-diff pins (no neg) should error")
	}
}

func TestValidateMixedDeviceAndPinErrors(t *testing.T) {
	// one device driver + one pin driver -> not all pin-context -> still an error
	sigs := map[string]*Signal{"s": {Name: "s", Type: &ResolvedType{Mark: "std_logic"}, Ports: []*SignalPortRef{
		{Context: Context{Kind: "device", ID: "d"}, Dir: "out", Type: &ResolvedType{Mark: "std_logic"}},
		{Context: Context{Kind: "pin", ID: "p"}, Dir: "out", Type: &ResolvedType{Mark: "std_logic"}, Element: "s.x(0)"},
	}}}
	if err := validateSignals(sigs); !errors.Is(err, ErrMultiDriver) {
		t.Error("mixed device+pin drivers should error")
	}
}

// TestErrorMessages is a smoke test for the .Error() rendering of each typed error.
func TestErrorMessages(t *testing.T) {
	re := &ResolveError{Kind: ErrEntityNotFound, Ctx: `class "spi"`, Name: "spidb"}
	if got := re.Error(); got != `class "spi": unable to map to entity "spidb"` {
		t.Errorf("ResolveError.Error() = %q", got)
	}
	se := &SignalError{Kind: ErrUndrivenSignal, Signal: "clk", Detail: "d0.clk"}
	if got := se.Error(); got != `nothing drives signal "clk" used by d0.clk` {
		t.Errorf("SignalError.Error() = %q", got)
	}
	ae := &AddrError{Kind: ErrBadRegion, Device: "flash", Base: 0xb0000000}
	if got := ae.Error(); got != `device "flash" base address 0xb0000000 is invalid: bits 31-28 must be 0xA` {
		t.Errorf("AddrError.Error() = %q", got)
	}
}
