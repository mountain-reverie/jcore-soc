package emit

import (
	"strings"
	"testing"

	"github.com/j-core/jcore-soc/tools/socgen/elaborate"
	"github.com/j-core/jcore-soc/tools/socgen/iface"
	"github.com/j-core/jcore-soc/tools/socgen/vhdl"
)

func TestBufEntityPinEmitsNoBufferNoAssign(t *testing.T) {
	res := &elaborate.Resolution{
		Target:          "ecp5",
		EntityBoundPads: map[string]bool{"sdram_d": true},
		Pins: []*elaborate.ResolvedPin{
			{Net: "sdram_d0", Pad: "J16", Signal: "sdram_d", PadDir: "inout", BufferKind: elaborate.BufEntity},
		},
	}
	stmts, err := pinStatements(res)
	if err != nil {
		t.Fatalf("pinStatements: %v", err)
	}
	out := printStmts(stmts)
	for _, bad := range []string{"IBUF", "OBUF", "IOBUF", "pin_sdram_d0 <=", "<= pin_sdram_d0"} {
		if strings.Contains(out, bad) {
			t.Errorf("BufEntity pad must emit no buffer/assign, found %q in:\n%s", bad, out)
		}
	}
	if len(stmts) != 0 {
		t.Errorf("expected 0 statements for a lone BufEntity pad, got %d", len(stmts))
	}
}

func TestEntityBoundPortMapsToPerBitPads(t *testing.T) {
	res := &elaborate.Resolution{
		Target:          "ecp5",
		EntityBoundPads: map[string]bool{"sdram_d": true},
		Pins: []*elaborate.ResolvedPin{
			{Net: "sdram_d0", Pad: "J16", Signal: "sdram_d", PadDir: "inout", BufferKind: elaborate.BufEntity},
			{Net: "sdram_d1", Pad: "L18", Signal: "sdram_d", PadDir: "inout", BufferKind: elaborate.BufEntity},
		},
	}
	re := &elaborate.ResolvedEntity{
		Name:   "sdram_iocells",
		Entity: &iface.Entity{Name: "sdram_iocells"},
		Ports: []*elaborate.ResolvedPort{
			{Name: "dq", Kind: elaborate.KindSignal, GlobalSignal: "sdram_d"},
		},
	}
	inst := topInstStmt(re, res)
	got := printStmts([]vhdl.Stmt{inst})
	for _, want := range []string{"dq(0) => pin_sdram_d0", "dq(1) => pin_sdram_d1"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}
