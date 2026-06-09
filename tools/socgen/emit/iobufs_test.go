package emit

import (
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
