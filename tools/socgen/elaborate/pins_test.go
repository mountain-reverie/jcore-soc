package elaborate

import (
	"errors"
	"testing"

	"github.com/j-core/jcore-soc/tools/socgen/design"
)

func TestValidatePinInvert(t *testing.T) {
	mk := func(leg string) *design.Design {
		r := &design.PinRule{Match: &design.Match{Regex: "x"}}
		s := &design.SigSpec{Kind: design.SigMap, Name: "foo", Invert: true}
		switch leg {
		case "signal":
			r.Signal = s
		case "in":
			r.In = s
		case "out-en":
			r.OutEn = s
		case "out":
			r.Out = s
		}
		return &design.Design{Pins: &design.PinsSpec{Rules: []*design.PinRule{r}}}
	}
	for _, leg := range []string{"signal", "in", "out-en"} {
		if err := validatePinInvert(mk(leg)); err == nil || !errors.Is(err, ErrUnsupportedInvert) {
			t.Errorf("%s invert: want ErrUnsupportedInvert, got %v", leg, err)
		}
	}
	if err := validatePinInvert(mk("out")); err != nil {
		t.Errorf("out invert should be allowed, got %v", err)
	}
}

func TestMatchPinRegexAndParametric(t *testing.T) {
	// regex full-match
	r := &design.PinRule{Match: &design.Match{Regex: "uart_tx"}}
	if env, ok := matchPin(r, "uart_tx"); !ok || len(env) != 0 {
		t.Errorf("regex match: ok=%v env=%v", ok, env)
	}
	if _, ok := matchPin(r, "uart_tx2"); ok {
		t.Error("regex should full-match (uart_tx2 must not match uart_tx)")
	}
	// parametric [mcb3_dram_a, n]
	p := &design.PinRule{Match: &design.Match{Parts: []design.SeqPart{{Lit: "mcb3_dram_a"}, {Sym: "n"}}}}
	env, ok := matchPin(p, "mcb3_dram_a12")
	if !ok || env["n"] != 12 {
		t.Errorf("parametric: ok=%v env=%v", ok, env)
	}
	// multi-part [io_p, n, "_", m]
	mp := &design.PinRule{Match: &design.Match{Parts: []design.SeqPart{{Lit: "io_p"}, {Sym: "n"}, {Lit: "_"}, {Sym: "m"}}}}
	env, ok = matchPin(mp, "io_p3_7")
	if !ok || env["n"] != 3 || env["m"] != 7 {
		t.Errorf("multi-part: ok=%v env=%v", ok, env)
	}
}

func TestExpandSig(t *testing.T) {
	env := map[string]int{"n": 5}
	tmpl := &design.SigSpec{Kind: design.SigTemplate, Parts: []design.SeqPart{{Lit: "dr_data_o.dqo("}, {Sym: "n"}, {Lit: ")"}}}
	ref, diff, kind := expandSig(tmpl, "thepin", env)
	if ref != "dr_data_o.dqo(5)" || diff != "" || kind != design.SigName {
		t.Errorf("template: ref=%q diff=%q kind=%v", ref, diff, kind)
	}
	if ref, _, _ := expandSig(&design.SigSpec{Kind: design.SigTrue}, "thepin", env); ref != "thepin" {
		t.Errorf("true: %q", ref)
	}
	if ref, diff, _ := expandSig(&design.SigSpec{Kind: design.SigMap, Name: "ddr_clk", Diff: "pos"}, "p", env); ref != "ddr_clk" || diff != "pos" {
		t.Errorf("map: ref=%q diff=%q", ref, diff)
	}
	if _, _, kind := expandSig(&design.SigSpec{Kind: design.SigConst, Int: 0}, "p", env); kind != design.SigConst {
		t.Errorf("const kind=%v", kind)
	}
	// SigName (the common scalar-string case) and nil
	if ref, _, kind := expandSig(&design.SigSpec{Kind: design.SigName, Name: "flash_cs(0)"}, "p", env); ref != "flash_cs(0)" || kind != design.SigName {
		t.Errorf("name: ref=%q kind=%v", ref, kind)
	}
	if ref, diff, kind := expandSig(nil, "p", env); ref != "" || diff != "" || kind != design.SigName {
		t.Errorf("nil: ref=%q diff=%q kind=%v", ref, diff, kind)
	}
}

func TestMatchPinNilMatch(t *testing.T) {
	if _, ok := matchPin(&design.PinRule{}, "any"); ok {
		t.Error("a rule with nil Match must not match")
	}
}

func TestFoldRulesTriStateLegs(t *testing.T) {
	rules := []*design.PinRule{
		{Match: &design.Match{Parts: []design.SeqPart{{Lit: "mcb3_dram_dq"}, {Sym: "n"}}},
			In:    &design.SigSpec{Kind: design.SigTemplate, Parts: []design.SeqPart{{Lit: "dr_data_i.dqi("}, {Sym: "n"}, {Lit: ")"}}},
			Out:   &design.SigSpec{Kind: design.SigTemplate, Parts: []design.SeqPart{{Lit: "dr_data_o.dqo("}, {Sym: "n"}, {Lit: ")"}}},
			OutEn: &design.SigSpec{Kind: design.SigTemplate, Parts: []design.SeqPart{{Lit: "dr_data_o.dq_outen("}, {Sym: "n"}, {Lit: ")"}}}},
		{Match: &design.Match{Regex: "mcb3_dram_dq8"}, Buff: func() *bool { b := false; return &b }()},
	}
	f := foldRules(rules, &design.Pin{Net: "mcb3_dram_dq8", Pad: "L2"})
	if f.inRef != "dr_data_i.dqi(8)" || f.outRef != "dr_data_o.dqo(8)" || f.outEnRef != "dr_data_o.dq_outen(8)" {
		t.Errorf("legs: in=%q out=%q outen=%q", f.inRef, f.outRef, f.outEnRef)
	}
	if f.buff == nil || *f.buff != false {
		t.Errorf("buff: %v", f.buff)
	}
}

func TestFoldRulesConstAndNoMatch(t *testing.T) {
	// Out: 0 (constant) -> outConst captured, no outRef
	cf := foldRules([]*design.PinRule{
		{Match: &design.Match{Regex: "eth_mdc"}, Out: &design.SigSpec{Kind: design.SigConst, Int: 0}},
	}, &design.Pin{Net: "eth_mdc"})
	if cf.outConst == nil || *cf.outConst != 0 || cf.outRef != "" {
		t.Errorf("const: outConst=%v outRef=%q", cf.outConst, cf.outRef)
	}
	// no rule matches -> zero folded with a non-nil empty attrs map
	nf := foldRules([]*design.PinRule{{Match: &design.Match{Regex: "other"}}}, &design.Pin{Net: "lonely"})
	if nf.attrs == nil || len(nf.attrs) != 0 || nf.signalRef != "" || nf.inRef != "" || nf.buff != nil {
		t.Errorf("no-match folded not zero: %+v", nf)
	}
}

func TestFoldRulesAttrsLastWins(t *testing.T) {
	rules := []*design.PinRule{
		{Match: &design.Match{Regex: ".*"}, Attrs: map[string]design.Value{"iostandard": {Kind: design.KindExpr, Text: "LVCMOS33"}}},
		{Match: &design.Match{Regex: "mcb3_dram_ck"}, Attrs: map[string]design.Value{"iostandard": {Kind: design.KindExpr, Text: "DIFF_MOBILE_DDR"}}},
		{Match: &design.Match{Regex: "mcb3_dram_ck"}, Signal: &design.SigSpec{Kind: design.SigMap, Name: "ddr_clk", Diff: "pos"}},
	}
	f := foldRules(rules, &design.Pin{Net: "mcb3_dram_ck", Pad: "E3"})
	if f.attrs["iostandard"].Text != "DIFF_MOBILE_DDR" {
		t.Errorf("attrs: %+v", f.attrs)
	}
	if f.signalRef != "ddr_clk" || f.signalDiff != "pos" {
		t.Errorf("signal: ref=%q diff=%q", f.signalRef, f.signalDiff)
	}
}

func TestSplitSignal(t *testing.T) {
	cases := map[string][2]string{ // ref -> [base, element]
		"clk":              {"clk", ""},
		"flash_cs(0)":      {"flash_cs", "flash_cs(0)"},
		"dr_data_o.dqo(3)": {"dr_data_o", "dr_data_o.dqo(3)"},
		"ddr_sd_ctrl.cke":  {"ddr_sd_ctrl", "ddr_sd_ctrl.cke"},
	}
	for ref, want := range cases {
		base, elem := splitSignal(ref)
		if base != want[0] || elem != want[1] {
			t.Errorf("splitSignal(%q) = (%q,%q) want (%q,%q)", ref, base, elem, want[0], want[1])
		}
	}
}

func TestBareSignalAutoDirection(t *testing.T) {
	// signal already driven (an out port present) -> pin consumes (dir "in")
	driven := map[string]*Signal{"s": {Name: "s", Ports: []*SignalPortRef{{Context: Context{Kind: "device", ID: "d"}, Dir: "out"}}}}
	if d := bareSignalDir(driven, "s"); d != "in" {
		t.Errorf("driven signal -> pin should consume (in), got %q", d)
	}
	// signal undriven -> pin drives (dir "out")
	undriven := map[string]*Signal{"s": {Name: "s", Ports: []*SignalPortRef{{Context: Context{Kind: "device", ID: "d"}, Dir: "in"}}}}
	if d := bareSignalDir(undriven, "s"); d != "out" {
		t.Errorf("undriven signal -> pin should drive (out), got %q", d)
	}
	// signal absent -> pin drives (out)
	if d := bareSignalDir(map[string]*Signal{}, "s"); d != "out" {
		t.Errorf("absent signal -> pin should drive (out), got %q", d)
	}
	// buffer and inout also count as existing drivers -> pin consumes (in)
	for _, drv := range []string{"buffer", "inout"} {
		sigs := map[string]*Signal{"s": {Name: "s", Ports: []*SignalPortRef{{Context: Context{Kind: "device", ID: "d"}, Dir: drv}}}}
		if d := bareSignalDir(sigs, "s"); d != "in" {
			t.Errorf("%s driver -> pin should consume (in), got %q", drv, d)
		}
	}
}

func TestResolvePinsDifferentialConsistentDirection(t *testing.T) {
	rules := []*design.PinRule{
		{Match: &design.Match{Regex: "ck_n"}, Signal: &design.SigSpec{Kind: design.SigMap, Name: "ddr_clk", Diff: "neg"}},
		{Match: &design.Match{Regex: "ck_p"}, Signal: &design.SigSpec{Kind: design.SigMap, Name: "ddr_clk", Diff: "pos"}},
	}
	mk := func() *design.Design {
		return &design.Design{Pins: &design.PinsSpec{Rules: rules, Pins: []*design.Pin{{Net: "ck_n", Pad: "A"}, {Net: "ck_p", Pad: "B"}}}}
	}
	// real net consumed by a device (in port) but not driven -> BOTH pins drive
	// (input pads) -> both IBUFDS, same direction. The net must be real (a device
	// declares it) or the bare-signal pins would be dropped as :missing.
	sigs := map[string]*Signal{"ddr_clk": {Name: "ddr_clk", Ports: []*SignalPortRef{{Context: Context{Kind: "device", ID: "ddrc"}, Dir: "in"}}}}
	pins := resolvePins(mk(), sigs)
	bufs := map[string]BufferKind{}
	for _, p := range pins {
		bufs[p.Net] = p.BufferKind
	}
	dirs := map[string]string{}
	for _, pr := range sigs["ddr_clk"].Ports {
		dirs[pr.Diff] = pr.Dir
	}
	if dirs["neg"] != dirs["pos"] {
		t.Errorf("differential pair must share direction: neg=%q pos=%q", dirs["neg"], dirs["pos"])
	}
	if bufs["ck_n"] != BufIBUFDS || bufs["ck_p"] != BufIBUFDS {
		t.Errorf("undriven differential -> both IBUFDS, got ck_n=%v ck_p=%v", bufs["ck_n"], bufs["ck_p"])
	}
	// device-driven net -> BOTH pins consume (output pads) -> both OBUFDS.
	sigs2 := map[string]*Signal{"ddr_clk": {Name: "ddr_clk", Ports: []*SignalPortRef{{Context: Context{Kind: "device", ID: "ddrc"}, Dir: "out"}}}}
	bufs2 := map[string]BufferKind{}
	for _, p := range resolvePins(mk(), sigs2) {
		bufs2[p.Net] = p.BufferKind
	}
	if bufs2["ck_n"] != BufOBUFDS || bufs2["ck_p"] != BufOBUFDS {
		t.Errorf("device-driven differential -> both OBUFDS, got ck_n=%v ck_p=%v", bufs2["ck_n"], bufs2["ck_p"])
	}
}

func TestResolvePinsJoinAndBuffer(t *testing.T) {
	d := &design.Design{
		Pins: &design.PinsSpec{
			Rules: []*design.PinRule{
				{Match: &design.Match{Regex: "clk"}, Signal: &design.SigSpec{Kind: design.SigTrue}},
				{Match: &design.Match{Regex: "led"}, Out: &design.SigSpec{Kind: design.SigName, Name: "po(0)"}},
				{Match: &design.Match{Regex: "ddr_ck"}, Signal: &design.SigSpec{Kind: design.SigMap, Name: "ddr_clk", Diff: "pos"}},
			},
			Pins: []*design.Pin{{Net: "clk", Pad: "V10"}, {Net: "led", Pad: "P4"}, {Net: "ddr_ck", Pad: "E3"}},
		},
	}
	// pre-seed sigs: 'ddr_clk' driven by a device (pin consumes it -> output pad
	// -> OBUFDS); 'clk' a real net consumed by a device (in port) but not driven,
	// so the pin drives it (-> input pad, IBUF). 'po' consumed by a device (gpio
	// output bus). All targets must be real nets or the pins would be dropped as
	// :missing — the generalized check now covers explicit in/out/out-en legs too.
	sigs := map[string]*Signal{
		"ddr_clk": {Name: "ddr_clk", Ports: []*SignalPortRef{{Context: Context{Kind: "device", ID: "ddrc"}, Dir: "out"}}},
		"clk":     {Name: "clk", Ports: []*SignalPortRef{{Context: Context{Kind: "device", ID: "clkgen"}, Dir: "in"}}},
		"po":      {Name: "po", Ports: []*SignalPortRef{{Context: Context{Kind: "device", ID: "gpio"}, Dir: "out"}}},
	}
	pins := resolvePins(d, sigs)
	// clk consumed by a device -> pin drives it (out), pin-context, IBUF
	var clkPin *SignalPortRef
	for _, p := range sigs["clk"].Ports {
		if p.Context.Kind == "pin" {
			clkPin = p
		}
	}
	if clkPin == nil || clkPin.Dir != "out" {
		t.Fatalf("clk join: %+v", sigs["clk"])
	}
	if clkPin.PortName != "pin.clk.signal" {
		t.Errorf("clk PortName = %q want pin.clk.signal", clkPin.PortName)
	}
	// led -> po(0): consumer of base 'po', element recorded; po now has a pre-seeded
	// device port + the new pin port, so find the pin-context port specifically.
	po := sigs["po"]
	var poPinRef *SignalPortRef
	if po != nil {
		for _, p := range po.Ports {
			if p.Context.Kind == ctxKindPin {
				poPinRef = p
				break // only one pin port expected; break makes the intent explicit
			}
		}
	}
	if poPinRef == nil || poPinRef.Dir != "in" || poPinRef.Element != "po(0)" {
		t.Fatalf("po join: %+v", po)
	}
	// ddr_ck differential: pin consumes already-driven ddr_clk (dir "in"), diff pos recorded
	dc := sigs["ddr_clk"]
	var pinRef *SignalPortRef
	for _, p := range dc.Ports {
		if p.Context.Kind == "pin" {
			pinRef = p
		}
	}
	if pinRef == nil || pinRef.Dir != "in" || pinRef.Diff != "pos" {
		t.Fatalf("ddr_clk pin ref: %+v", dc.Ports)
	}
	byNet := map[string]*ResolvedPin{}
	for _, p := range pins {
		byNet[p.Net] = p
	}
	if byNet["clk"].BufferKind != BufIBUF {
		t.Errorf("clk buffer = %v want IBUF", byNet["clk"].BufferKind)
	}
	if byNet["led"].BufferKind != BufOBUF {
		t.Errorf("led buffer = %v want OBUF", byNet["led"].BufferKind)
	}
	// bare differential consumed by design -> output differential pad
	if byNet["ddr_ck"].BufferKind != BufOBUFDS {
		t.Errorf("ddr_ck buffer = %v want OBUFDS", byNet["ddr_ck"].BufferKind)
	}
}

func TestBufferKindDirect(t *testing.T) {
	bf := false
	if k := bufferKind(folded{buff: &bf, signalRef: "x"}, "out"); k != BufDirect {
		t.Errorf("buff:false -> %v want Direct", k)
	}
}

func TestSignalIsReal(t *testing.T) {
	sigs := map[string]*Signal{
		"real": {Name: "real", Ports: []*SignalPortRef{
			{Context: Context{Kind: "device", ID: "u_dev"}, Dir: dirOut},
		}},
		"pinonly": {Name: "pinonly", Ports: []*SignalPortRef{
			{Context: Context{Kind: ctxKindPin, ID: "somepin"}, Dir: dirIn},
		}},
		"zpin": {Name: "zpin", Ports: []*SignalPortRef{ // pin-only AND a zero-signal
			{Context: Context{Kind: ctxKindPin, ID: "somepin"}, Dir: dirIn},
		}},
	}
	d := &design.Design{ZeroSignals: []string{"zsig", "zpin"}}

	if !signalIsReal(sigs, d, "real") {
		t.Errorf("a signal with a device port should be real")
	}
	if signalIsReal(sigs, d, "pinonly") {
		t.Errorf("a signal with only pin ports should NOT be real")
	}
	if signalIsReal(sigs, d, "absent") {
		t.Errorf("a signal not in the net-list should NOT be real")
	}
	if !signalIsReal(sigs, d, "zsig") {
		t.Errorf("a declared zero-signal should be real")
	}
	if !signalIsReal(sigs, d, "zpin") {
		t.Errorf("a zero-signal with only pin ports should still be real (zero-signal wins)")
	}
}

func TestPadDir(t *testing.T) {
	cases := []struct {
		bk      BufferKind
		bareDir string
		want    string
	}{
		{BufIBUF, "", dirIn}, {BufIBUFDS, "", dirIn},
		{BufOBUF, "", dirOut}, {BufOBUFT, "", dirOut}, {BufOBUFDS, "", dirOut},
		{BufIOBUF, "", dirInout},
		{BufDirect, dirOut, dirIn}, // pin drives net -> input pad
		{BufDirect, dirIn, dirOut},
	}
	for _, c := range cases {
		if g := padDir(c.bk, c.bareDir); g != c.want {
			t.Errorf("padDir(%v,%q) = %q, want %q", c.bk, c.bareDir, g, c.want)
		}
	}
}
