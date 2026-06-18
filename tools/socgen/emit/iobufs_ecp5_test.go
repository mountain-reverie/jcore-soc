package emit

import (
	"strings"
	"testing"

	"github.com/j-core/jcore-soc/tools/socgen/elaborate"
	"github.com/j-core/jcore-soc/tools/socgen/vhdl"
)

func printStmts(stmts []vhdl.Stmt) string {
	df := &vhdl.DesignFile{
		Units: []vhdl.DesignUnit{
			&vhdl.ArchitectureBody{Name: "impl", Entity: "pad_ring", Stmts: stmts},
		},
	}
	return vhdl.Print(df)
}

// ecp5TestPins returns pins that the DEFAULT (Xilinx) path wraps in OBUF/IBUF
// (BufferKind is set, not the zero-value BufDirect). That is what makes the
// ecp5 assertions meaningful: only the ecp5 branch turns these into
// buffer-free direct assigns. The control test below relies on the same pins.
func ecp5TestPins() []*elaborate.ResolvedPin {
	return []*elaborate.ResolvedPin{
		{Net: "led0", Pad: "B2", Out: "led_o0", PadDir: "out", BufferKind: elaborate.BufOBUF},
		{Net: "uart_rx", Pad: "M1", In: "rx_i", PadDir: "in", BufferKind: elaborate.BufIBUF},
	}
}

func TestEcp5PinStatementsNoBuffers(t *testing.T) {
	res := &elaborate.Resolution{Target: "ecp5", Pins: ecp5TestPins()}
	stmts, err := pinStatements(res)
	if err != nil {
		t.Fatalf("pinStatements: %v", err)
	}
	out := printStmts(stmts)
	for _, bad := range []string{"IBUF", "OBUF", "IOBUF", "unisim"} {
		if strings.Contains(out, bad) {
			t.Errorf("ecp5 output must not contain %q:\n%s", bad, out)
		}
	}
	if !strings.Contains(out, "pin_led0 <= led_o0") {
		t.Errorf("missing output pad assign; got:\n%s", out)
	}
	if !strings.Contains(out, "rx_i <= pin_uart_rx") {
		t.Errorf("missing input pad assign; got:\n%s", out)
	}
}

// TestNonEcp5PinStatementsKeepBuffers is the control: the SAME pins on a
// non-ecp5 target still emit vendor buffers. This proves the "no buffers"
// assertions above are non-trivial — without the ecp5 branch they would fail.
func TestNonEcp5PinStatementsKeepBuffers(t *testing.T) {
	res := &elaborate.Resolution{Target: "spartan6", Pins: ecp5TestPins()}
	stmts, err := pinStatements(res)
	if err != nil {
		t.Fatalf("pinStatements: %v", err)
	}
	out := printStmts(stmts)
	if !strings.Contains(out, "OBUF") || !strings.Contains(out, "IBUF") {
		t.Errorf("non-ecp5 path must still instantiate vendor buffers; got:\n%s", out)
	}
}

// TestEcp5PinStatementsBareSignal covers bare-signal pins (Signal set, no
// In/Out legs). Direction follows PadDir, mirroring the Xilinx BufDirect
// convention: "in" drives the net; an unset PadDir drives the pad (output).
func TestEcp5PinStatementsBareSignal(t *testing.T) {
	res := &elaborate.Resolution{
		Target: "ecp5",
		Pins: []*elaborate.ResolvedPin{
			{Net: "clk", Pad: "G2", Signal: "clk_net", PadDir: "in"}, // input: net <= pad
			{Net: "dbg", Pad: "H3", Signal: "dbg_net"},               // unset PadDir: output (pad <= net)
		},
	}
	stmts, err := pinStatements(res)
	if err != nil {
		t.Fatalf("pinStatements: %v", err)
	}
	out := printStmts(stmts)
	if !strings.Contains(out, "clk_net <= pin_clk") {
		t.Errorf("bare-signal input should drive the net; got:\n%s", out)
	}
	if !strings.Contains(out, "pin_dbg <= dbg_net") {
		t.Errorf("bare-signal with empty PadDir should drive the pad (output), matching Xilinx BufDirect; got:\n%s", out)
	}
}
