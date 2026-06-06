package elaborate

import (
	"testing"

	"github.com/j-core/jcore-soc/tools/socgen/design"
)

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
	// Out: 0 (constant) -> hasConst set, no outRef
	cf := foldRules([]*design.PinRule{
		{Match: &design.Match{Regex: "eth_mdc"}, Out: &design.SigSpec{Kind: design.SigConst, Int: 0}},
	}, &design.Pin{Net: "eth_mdc"})
	if !cf.hasConst || cf.outRef != "" {
		t.Errorf("const: hasConst=%v outRef=%q", cf.hasConst, cf.outRef)
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
