package emit

import (
	"strings"
	"testing"

	"github.com/j-core/jcore-soc/tools/socgen/elaborate"
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
