package design

import (
	"testing"

	"gopkg.in/yaml.v3"
)

func decodePins(t *testing.T, src string) *PinsSpec {
	t.Helper()
	var d Design
	if err := yaml.Unmarshal([]byte(src), &d); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if d.Pins == nil {
		t.Fatal("Pins nil")
	}
	return d.Pins
}

func TestPinsUnmarshalRuleShapes(t *testing.T) {
	ps := decodePins(t, `
pins:
  file: ../pins/x.pins
  type: pin-names
  part: IC3
  rules:
    - { match: ".*", attrs: { iostandard: LVCMOS33 } }
    - { match: clk_100mhz, signal: true, buff: false }
    - { match: uart_tx, signal: uart0_rx }
    - { match: sd_cs, signal: flash_cs(0) }
    - { match: mcb3_dram_ck, signal: { name: ddr_clk, diff: pos } }
    - { match: [mcb3_dram_a, n], signal: ["ddr_sd_ctrl.a(", n, ")"] }
    - match: [mcb3_dram_dq, n]
      in: ["dr_data_i.dqi(", n, ")"]
      out: ["dr_data_o.dqo(", n, ")"]
      out-en: ["dr_data_o.dq_outen(", n, ")"]
    - match: [io_p, n, "_", m]
      signal: ["io_p", n, "(", m, ")"]
    - { match: eth_mdc, out: 0 }
`)
	if ps.File != "../pins/x.pins" || ps.Type != "pin-names" || ps.Part != "IC3" {
		t.Fatalf("spec header: %+v", ps)
	}
	if len(ps.Rules) != 9 {
		t.Fatalf("rules: %d", len(ps.Rules))
	}
	if ps.Rules[0].Match.Regex != ".*" || ps.Rules[0].Attrs["iostandard"].Text != "LVCMOS33" {
		t.Errorf("rule0: %+v", ps.Rules[0])
	}
	if ps.Rules[1].Signal.Kind != SigTrue || ps.Rules[1].Buff == nil || *ps.Rules[1].Buff != false {
		t.Errorf("rule1: %+v", ps.Rules[1])
	}
	if ps.Rules[2].Signal.Kind != SigName || ps.Rules[2].Signal.Name != "uart0_rx" {
		t.Errorf("rule2: %+v", ps.Rules[2])
	}
	if ps.Rules[3].Signal.Kind != SigName || ps.Rules[3].Signal.Name != "flash_cs(0)" {
		t.Errorf("rule3: %+v", ps.Rules[3])
	}
	if ps.Rules[4].Signal.Kind != SigMap || ps.Rules[4].Signal.Name != "ddr_clk" || ps.Rules[4].Signal.Diff != "pos" {
		t.Errorf("rule4: %+v", ps.Rules[4])
	}
	m := ps.Rules[5].Match
	if m.Regex != "" || len(m.Parts) != 2 || m.Parts[0].Lit != "mcb3_dram_a" || m.Parts[1].Sym != "n" {
		t.Errorf("rule5 match: %+v", m)
	}
	sig := ps.Rules[5].Signal
	if sig.Kind != SigTemplate || len(sig.Parts) != 3 || sig.Parts[0].Lit != "ddr_sd_ctrl.a(" || sig.Parts[1].Sym != "n" || sig.Parts[2].Lit != ")" {
		t.Errorf("rule5 signal: %+v", sig)
	}
	r6 := ps.Rules[6]
	if r6.In == nil || r6.Out == nil || r6.OutEn == nil {
		t.Fatalf("rule6 keys: %+v", r6)
	}
	if r6.Out.Kind != SigTemplate || r6.Out.Parts[0].Lit != "dr_data_o.dqo(" {
		t.Errorf("rule6 out: %+v", r6.Out)
	}
	if r6.In.Parts[0].Lit != "dr_data_i.dqi(" || r6.In.Parts[1].Sym != "n" {
		t.Errorf("rule6 in: %+v", r6.In)
	}
	if r6.OutEn.Parts[0].Lit != "dr_data_o.dq_outen(" || r6.OutEn.Parts[1].Sym != "n" {
		t.Errorf("rule6 out-en: %+v", r6.OutEn)
	}
	m7 := ps.Rules[7].Match
	if len(m7.Parts) != 4 || m7.Parts[0].Lit != "io_p" || m7.Parts[1].Sym != "n" || m7.Parts[2].Lit != "_" || m7.Parts[3].Sym != "m" {
		t.Errorf("rule7 match: %+v", m7)
	}
	s7 := ps.Rules[7].Signal
	if s7.Kind != SigTemplate || len(s7.Parts) != 5 || s7.Parts[0].Lit != "io_p" || s7.Parts[1].Sym != "n" || s7.Parts[2].Lit != "(" || s7.Parts[3].Sym != "m" || s7.Parts[4].Lit != ")" {
		t.Errorf("rule7 signal: %+v", s7)
	}
	if ps.Rules[8].Out.Kind != SigConst || ps.Rules[8].Out.Int != 0 {
		t.Errorf("rule8 out: %+v", ps.Rules[8].Out)
	}
}

func TestParsePinNames(t *testing.T) {
	src := "# comment\nCLK_100MHz V10\n\nUART_TX A8\nmcb3_dram_a0  J7\n"
	pins, errs := parsePinNames([]byte(src))
	if len(errs) != 0 {
		t.Fatalf("errs: %v", errs)
	}
	want := []Pin{{"clk_100mhz", "V10"}, {"uart_tx", "A8"}, {"mcb3_dram_a0", "J7"}}
	if len(pins) != len(want) {
		t.Fatalf("got %d pins: %+v", len(pins), pins)
	}
	for i, w := range want {
		if *pins[i] != w {
			t.Errorf("pin %d = %+v want %+v", i, *pins[i], w)
		}
	}
}

func TestParsePinListEagle(t *testing.T) {
	src := `Pinlist

Part     Pad      Pin        Dir      Net

IC3      A1       GND        io       GND
         A2       IO_L2N_0   io       USB0_N
         A3       IO_L4N_0   io       !VID-EN
         A7       IO_L10N_0  io                *** unconnected ***
         A8       IO_L33N_0  io       HDMI-D2_N
`
	pins, errs := parsePinList([]byte(src), "IC3")
	if len(errs) != 0 {
		t.Fatalf("errs: %v", errs)
	}
	// unconnected dropped; net normalized: lower, '-'->'_', '!' removed
	want := []Pin{{"gnd", "A1"}, {"usb0_n", "A2"}, {"vid_en", "A3"}, {"hdmi_d2_n", "A8"}}
	if len(pins) != len(want) {
		t.Fatalf("got %d pins: %+v", len(pins), pins)
	}
	for i, w := range want {
		if *pins[i] != w {
			t.Errorf("pin %d = %+v want %+v", i, *pins[i], w)
		}
	}
}

func TestParsePinListPartNotFound(t *testing.T) {
	pins, errs := parsePinList([]byte("Part Pad Pin Dir Net\nIC9 A1 GND io GND\n"), "IC3")
	if len(pins) != 0 || len(errs) != 1 {
		t.Fatalf("expected one error and no pins: pins=%+v errs=%v", pins, errs)
	}
}
