package emit

import (
	"strings"
	"testing"

	"github.com/j-core/jcore-soc/tools/socgen/elaborate"
	"github.com/j-core/jcore-soc/tools/socgen/vhdl"
)

// renderPinStmts calls ecp5PinStmt and renders all returned statements to VHDL text.
func renderPinStmts(t *testing.T, rp *elaborate.ResolvedPin) (string, error) {
	t.Helper()
	stmts, err := ecp5PinStmt(rp)
	if err != nil {
		return "", err
	}
	return printStmts(stmts), nil
}

func TestEcp5PinDeviceAndSignalLegsBothEmitted(t *testing.T) {
	// btn0: input pad feeding gpio_di(0) AND the internal ext_rst signal.
	rp := &elaborate.ResolvedPin{Net: "btn0", Pad: "D6", In: "gpio_di(0)", Signal: "ext_rst", PadDir: "in"}
	out, err := renderPinStmts(t, rp)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"gpio_di(0) <= pin_btn0", "ext_rst <= pin_btn0"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}

func TestEcp5PinSingleLegUnchanged(t *testing.T) {
	cases := []struct {
		rp   *elaborate.ResolvedPin
		want string
	}{
		{&elaborate.ResolvedPin{Net: "btn1", In: "gpio_di(1)", PadDir: "in"}, "gpio_di(1) <= pin_btn1"},
		{&elaborate.ResolvedPin{Net: "led0", Out: "gpio_do(0)", PadDir: "out"}, "pin_led0 <= gpio_do(0)"},
		{&elaborate.ResolvedPin{Net: "clk_25mhz", Signal: "clk_25mhz", PadDir: "in"}, "clk_25mhz <= pin_clk_25mhz"},
	}
	for _, c := range cases {
		out, err := renderPinStmts(t, c.rp)
		if err != nil {
			t.Fatalf("%s: %v", c.rp.Net, err)
		}
		if !strings.Contains(out, c.want) {
			t.Errorf("%s: want %q, got:\n%s", c.rp.Net, c.want, out)
		}
	}
}

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

// TestEcp5PinStatementsInvertedOutput asserts that an inverted output pad on the
// ecp5/ice40 path emits `pin_x <= not <src>` directly (no intermediate signal).
func TestEcp5PinStatementsInvertedOutput(t *testing.T) {
	res := &elaborate.Resolution{
		Target: "ecp5",
		Pins: []*elaborate.ResolvedPin{
			{Net: "ledr_n", Pad: "B3", Out: "gpio_do(0)", OutInvert: true, PadDir: "out", BufferKind: elaborate.BufOBUF},
		},
	}
	stmts, err := pinStatements(res)
	if err != nil {
		t.Fatalf("pinStatements: %v", err)
	}
	out := printStmts(stmts)
	if !strings.Contains(out, "pin_ledr_n <= not gpio_do(0)") {
		t.Errorf("inverted output should emit `pin_ledr_n <= not gpio_do(0)`; got:\n%s", out)
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
