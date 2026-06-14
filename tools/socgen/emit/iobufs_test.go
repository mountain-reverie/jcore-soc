package emit

import (
	"errors"
	"strings"
	"testing"

	"github.com/j-core/jcore-soc/tools/socgen/design"
	"github.com/j-core/jcore-soc/tools/socgen/elaborate"
	"github.com/j-core/jcore-soc/tools/socgen/vhdl"
)

func TestBufferGenerics(t *testing.T) {
	attrs := map[string]design.Value{
		"drive":      {Kind: design.KindInt, Int: 24},
		"iostandard": {Kind: design.KindExpr, Text: "LVCMOS33"},
		"slew":       {Kind: design.KindExpr, Text: "fast"},
		"tig":        {Kind: design.KindExpr, Text: "yes"}, // NOT a buffer generic
	}
	gens := bufferGenerics(attrs, elaborate.BufOBUF)
	got := assocPairs(gens)
	want := [][2]string{{"DRIVE", "24"}, {"IOSTANDARD", `"LVCMOS33"`}, {"SLEW", `"fast"`}}
	if len(got) != len(want) {
		t.Fatalf("OBUF generics = %v, want %v", got, want)
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("OBUF generic[%d] = %v, want %v", i, got[i], w)
		}
	}
	ib := assocPairs(bufferGenerics(attrs, elaborate.BufIBUF))
	if len(ib) != 1 || ib[0] != [2]string{"IOSTANDARD", `"LVCMOS33"`} {
		t.Errorf("IBUF generics = %v, want [[IOSTANDARD \"LVCMOS33\"]]", ib)
	}
}

func TestConnExprs(t *testing.T) {
	rp := &elaborate.ResolvedPin{Signal: "sd_miso"}
	if renderExprStr(t, inExpr(rp)) != "sd_miso" || renderExprStr(t, outExpr(rp)) != "sd_miso" {
		t.Errorf("bare-signal fallback wrong")
	}
	rp2 := &elaborate.ResolvedPin{Out: "dr_data_o.dqo(0)", In: "dr_data_i.dqi(0)"}
	if renderExprStr(t, outExpr(rp2)) != "dr_data_o.dqo(0)" {
		t.Errorf("outExpr explicit leg wrong: %q", renderExprStr(t, outExpr(rp2)))
	}
	if renderExprStr(t, inExpr(rp2)) != "dr_data_i.dqi(0)" {
		t.Errorf("inExpr explicit leg wrong")
	}
	// OutConst: outExpr returns the literal, not a signal name.
	rpConst := &elaborate.ResolvedPin{OutConst: "'1'"}
	if got := renderExprStr(t, outExpr(rpConst)); got != "'1'" {
		t.Errorf("OutConst outExpr = %q, want '1'", got)
	}
	// OutInvert: outExpr returns the pad_<base>_n intermediate.
	rpInv := &elaborate.ResolvedPin{Out: "reset", OutInvert: true}
	if got := renderExprStr(t, outExpr(rpInv)); got != "pad_reset_n" {
		t.Errorf("OutInvert outExpr = %q, want pad_reset_n", got)
	}
}

func TestInvertSignalName(t *testing.T) {
	for _, c := range []struct{ ref, want string }{
		{"reset", "pad_reset_n"},
		{"po(0)", "pad_po_0_n"},
		{"bus.data(3)", "pad_bus_data_3_n"},
	} {
		if got := invertSignalName(c.ref); got != c.want {
			t.Errorf("invertSignalName(%q) = %q, want %q", c.ref, got, c.want)
		}
	}
}

func TestInstBuf(t *testing.T) {
	inst := instBuf("obuf_led0", "OBUF",
		[]*vhdl.AssocElement{{Formal: "I", Actual: &vhdl.Ident{Name: "po(0)"}}, {Formal: "O", Actual: &vhdl.Ident{Name: "pin_led0"}}},
		[]*vhdl.AssocElement{{Formal: "DRIVE", Actual: &vhdl.BasicLit{Kind: vhdl.INT, Value: "24"}}})
	out := renderStmt(t, inst)
	for _, want := range []string{"obuf_led0 : OBUF", "I => po(0)", "O => pin_led0", "DRIVE => 24"} {
		if !strings.Contains(out, want) {
			t.Errorf("instBuf missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "component OBUF") || strings.Contains(out, "entity") {
		t.Errorf("instBuf must be a BARE component instance, got:\n%s", out)
	}
}

func TestPinStmtSingleEnded(t *testing.T) {
	cases := []struct {
		name string
		rp   *elaborate.ResolvedPin
		want []string
	}{
		{"obuf", &elaborate.ResolvedPin{Net: "led0", BufferKind: elaborate.BufOBUF, Out: "po(0)",
			Attrs: map[string]design.Value{"drive": {Kind: design.KindInt, Int: 24}, "iostandard": {Kind: design.KindExpr, Text: "LVCMOS33"}}},
			[]string{"obuf_led0 : OBUF", "I => po(0)", "O => pin_led0", "DRIVE => 24", `IOSTANDARD => "LVCMOS33"`}},
		{"ibuf", &elaborate.ResolvedPin{Net: "sd_miso", BufferKind: elaborate.BufIBUF, Signal: "sd_miso",
			Attrs: map[string]design.Value{"iostandard": {Kind: design.KindExpr, Text: "LVCMOS33"}}},
			[]string{"ibuf_sd_miso : IBUF", "I => pin_sd_miso", "O => sd_miso", `IOSTANDARD => "LVCMOS33"`}},
		{"obuft", &elaborate.ResolvedPin{Net: "mcb3_dram_ldm", BufferKind: elaborate.BufOBUFT, Out: "dr_data_o.dmo(0)", OutEn: "dr_data_o.dq_outen(16)",
			Attrs: map[string]design.Value{"iostandard": {Kind: design.KindExpr, Text: "MOBILE_DDR"}}},
			[]string{"obuft_mcb3_dram_ldm : OBUFT", "I => dr_data_o.dmo(0)", "T => dr_data_o.dq_outen(16)", "O => pin_mcb3_dram_ldm"}},
		{"iobuf", &elaborate.ResolvedPin{Net: "mcb3_dram_dq0", BufferKind: elaborate.BufIOBUF, In: "dr_data_i.dqi(0)", Out: "dr_data_o.dqo(0)", OutEn: "dr_data_o.dq_outen(0)",
			Attrs: map[string]design.Value{"iostandard": {Kind: design.KindExpr, Text: "MOBILE_DDR"}}},
			[]string{"iobuf_mcb3_dram_dq0 : IOBUF", "I => dr_data_o.dqo(0)", "T => dr_data_o.dq_outen(0)", "O => dr_data_i.dqi(0)", "IO => pin_mcb3_dram_dq0"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			st, err := pinStmt(c.rp)
			if err != nil || st == nil {
				t.Fatalf("pinStmt(%s) = %v, %v", c.name, st, err)
			}
			out := renderStmt(t, st)
			for _, w := range c.want {
				if !strings.Contains(out, w) {
					t.Errorf("%s missing %q:\n%s", c.name, w, out)
				}
			}
		})
	}
}

func TestPinStmtDirectWire(t *testing.T) {
	in := &elaborate.ResolvedPin{Net: "clk_100mhz", BufferKind: elaborate.BufDirect, Signal: "clk_100mhz", PadDir: "in"}
	st, err := pinStmt(in)
	if err != nil {
		t.Fatalf("pinStmt(in): %v", err)
	}
	if got := renderStmt(t, st); !strings.Contains(got, "clk_100mhz <= pin_clk_100mhz") {
		t.Errorf("direct-wire input = %q", got)
	}
	outp := &elaborate.ResolvedPin{Net: "foo", BufferKind: elaborate.BufDirect, Out: "bar", PadDir: "out"}
	st2, err := pinStmt(outp)
	if err != nil {
		t.Fatalf("pinStmt(out): %v", err)
	}
	if got := renderStmt(t, st2); !strings.Contains(got, "pin_foo <= bar") {
		t.Errorf("direct-wire output = %q", got)
	}
}

func TestDiffPairsOBUFDS(t *testing.T) {
	pos := &elaborate.ResolvedPin{Net: "mcb3_dram_ck", BufferKind: elaborate.BufOBUFDS, Signal: "ddr_clk", Diff: "pos",
		Attrs: map[string]design.Value{"iostandard": {Kind: design.KindExpr, Text: "DIFF_MOBILE_DDR"}}}
	neg := &elaborate.ResolvedPin{Net: "mcb3_dram_ck_n", BufferKind: elaborate.BufOBUFDS, Signal: "ddr_clk", Diff: "neg",
		Attrs: map[string]design.Value{"iostandard": {Kind: design.KindExpr, Text: "DIFF_MOBILE_DDR"}}}
	stmts, err := diffPairs([]*elaborate.ResolvedPin{neg, pos}) // unsorted on purpose
	if err != nil {
		t.Fatalf("diffPairs: %v", err)
	}
	if len(stmts) != 1 {
		t.Fatalf("want 1 OBUFDS, got %d", len(stmts))
	}
	out := renderStmt(t, stmts[0])
	for _, w := range []string{"obufds_mcb3_dram_ck_mcb3_dram_ck_n : OBUFDS", "I => ddr_clk", "O => pin_mcb3_dram_ck", "OB => pin_mcb3_dram_ck_n", `IOSTANDARD => "DIFF_MOBILE_DDR"`} {
		if !strings.Contains(out, w) {
			t.Errorf("OBUFDS missing %q:\n%s", w, out)
		}
	}
}

func TestDiffPairsMissingLeg(t *testing.T) {
	lone := &elaborate.ResolvedPin{Net: "x", BufferKind: elaborate.BufOBUFDS, Signal: "s", Diff: "pos"}
	stmts, err := diffPairs([]*elaborate.ResolvedPin{lone})
	if !errors.Is(err, ErrDiffPair) {
		t.Errorf("want ErrDiffPair for a lone leg, got %v", err)
	}
	if len(stmts) != 0 {
		t.Errorf("want 0 stmts for incomplete pair, got %d", len(stmts))
	}
}

func TestPinStatementsRoundTrip(t *testing.T) {
	res := &elaborate.Resolution{
		Pins: []*elaborate.ResolvedPin{
			{Net: "led0", BufferKind: elaborate.BufOBUF, Out: "po(0)", PadDir: "out",
				Attrs: map[string]design.Value{"drive": {Kind: design.KindInt, Int: 24}, "iostandard": {Kind: design.KindExpr, Text: "LVCMOS33"}}},
			{Net: "clk_100mhz", BufferKind: elaborate.BufDirect, Signal: "clk_100mhz", PadDir: "in"},
			{Net: "mcb3_dram_ck", BufferKind: elaborate.BufOBUFDS, Signal: "ddr_clk", Diff: "pos", Attrs: map[string]design.Value{"iostandard": {Kind: design.KindExpr, Text: "DIFF_MOBILE_DDR"}}},
			{Net: "mcb3_dram_ck_n", BufferKind: elaborate.BufOBUFDS, Signal: "ddr_clk", Diff: "neg", Attrs: map[string]design.Value{"iostandard": {Kind: design.KindExpr, Text: "DIFF_MOBILE_DDR"}}},
		},
	}
	stmts, err := pinStatements(res)
	if err != nil {
		t.Fatalf("pinStatements: %v", err)
	}
	if len(stmts) != 3 { // 1 obuf + 1 direct-wire + 1 obufds
		t.Errorf("want 3 statements, got %d", len(stmts))
	}
}

func TestPadRingHasBuffers(t *testing.T) {
	res := &elaborate.Resolution{
		Pins: []*elaborate.ResolvedPin{
			{Net: "led0", Pad: "t18", PadDir: "out", BufferKind: elaborate.BufOBUF, Out: "po(0)",
				Attrs: map[string]design.Value{"drive": {Kind: design.KindInt, Int: 24}, "iostandard": {Kind: design.KindExpr, Text: "LVCMOS33"}}},
		},
		SignalLocations: &elaborate.SignalLocations{},
	}
	out, err := PadRing(res)
	if err != nil {
		t.Fatalf("PadRing: %v", err)
	}
	if _, perr := vhdl.ParseFile(vhdl.NewFileSet(), "pad_ring.vhd", []byte(out)); perr != nil {
		t.Fatalf("re-parse: %v\n%s", perr, out)
	}
	for _, w := range []string{"library unisim;", "use unisim.vcomponents.all;", "obuf_led0 : OBUF", "I => po(0)", "O => pin_led0"} {
		if !strings.Contains(out, w) {
			t.Errorf("PadRing missing %q:\n%s", w, out)
		}
	}
}

// --- test helpers ---

func assocPairs(gens []*vhdl.AssocElement) [][2]string {
	out := make([][2]string, 0, len(gens))
	for _, g := range gens {
		out = append(out, [2]string{g.Formal, renderExprTH(g.Actual)})
	}
	return out
}

func renderExprStr(t *testing.T, e vhdl.Expr) string { t.Helper(); return renderExprTH(e) }

func renderExprTH(e vhdl.Expr) string {
	df := &vhdl.DesignFile{Units: []vhdl.DesignUnit{
		&vhdl.ArchitectureBody{Name: "i", Entity: "e", Stmts: []vhdl.Stmt{
			&vhdl.ConcurrentSignalAssign{Target: &vhdl.Ident{Name: "x"}, Waveform: []*vhdl.WaveformElem{{Value: e}}},
		}},
	}}
	s := vhdl.Print(df)
	i := strings.Index(s, "x <= ")
	if i < 0 {
		return s
	}
	rest := s[i+len("x <= "):]
	j := strings.Index(rest, ";")
	if j < 0 {
		return strings.TrimSpace(rest)
	}
	return strings.TrimSpace(rest[:j])
}

func renderStmt(t *testing.T, st vhdl.Stmt) string {
	t.Helper()
	df := &vhdl.DesignFile{Units: []vhdl.DesignUnit{
		&vhdl.ArchitectureBody{Name: "i", Entity: "e", Stmts: []vhdl.Stmt{st}},
	}}
	return vhdl.Print(df)
}
