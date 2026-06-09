package emit

import (
	"strconv"
	"strings"

	"github.com/j-core/jcore-soc/tools/socgen/design"
	"github.com/j-core/jcore-soc/tools/socgen/elaborate"
	"github.com/j-core/jcore-soc/tools/socgen/vhdl"
)

// bufferGenericOrder is the fixed set of attributes that become I/O-buffer
// generic-map entries, in their sorted (emission) order. Each has a known VHDL
// type: drive is an integer, the rest are strings — so the value is rendered
// typed (drive bare, the others quoted), faithful to iobufs.clj gen-attrs.
// Must stay in sync with bufferGenericAttrs (padring.go), which excludes the
// same set from the pad-port attributes.
var bufferGenericOrder = []string{"diff_term", "drive", "iostandard", "slew"}

// bufferGenerics renders a pin's buffer generics as a sorted generic map. IBUF/
// IBUFDS additionally drop drive and slew (faithful to create-ibuf's dissoc).
func bufferGenerics(attrs map[string]design.Value, kind elaborate.BufferKind) []*vhdl.AssocElement {
	dropDriveSlew := kind == elaborate.BufIBUF || kind == elaborate.BufIBUFDS
	out := make([]*vhdl.AssocElement, 0, len(bufferGenericOrder))
	for _, name := range bufferGenericOrder {
		if dropDriveSlew && (name == "drive" || name == "slew") {
			continue
		}
		v, ok := attrs[name]
		if !ok {
			continue
		}
		out = append(out, &vhdl.AssocElement{Formal: strings.ToUpper(name), Actual: bufferGenericVal(name, v)})
	}
	return out
}

// bufferGenericVal renders a buffer-generic value with its known type: drive ->
// bare integer literal; iostandard/slew/diff_term -> quoted (escaped) string.
// All YAML kinds (KindExpr/KindStr) of a non-drive attribute render as a quoted
// VHDL string literal — the IOB string generics always take a string literal in
// FPGA synthesis, faithfully replicating gen-attrs in iobufs.clj (this is the
// deliberate departure from emitValue, which leaves a KindExpr scalar bare).
func bufferGenericVal(name string, v design.Value) vhdl.Expr {
	if name == "drive" {
		return &vhdl.BasicLit{Kind: vhdl.INT, Value: driveText(v)}
	}
	return &vhdl.BasicLit{Kind: vhdl.STRINGLIT, Value: `"` + vhdlEscape(v.Text) + `"`}
}

// driveText is the integer text of a drive value (KindInt -> the int; else the
// raw text).
func driveText(v design.Value) string {
	if v.Kind == design.KindInt {
		return strconv.FormatInt(v.Int, 10)
	}
	return v.Text
}

// outExpr / inExpr resolve a pin's internal-signal expression for a buffer port:
// the explicit out/in leg when present, else the bare signal.
func outExpr(rp *elaborate.ResolvedPin) vhdl.Expr {
	if rp.Out != "" {
		return &vhdl.Ident{Name: rp.Out}
	}
	return &vhdl.Ident{Name: rp.Signal}
}

func inExpr(rp *elaborate.ResolvedPin) vhdl.Expr {
	if rp.In != "" {
		return &vhdl.Ident{Name: rp.In}
	}
	return &vhdl.Ident{Name: rp.Signal}
}

// pinStmt builds the statement wiring one single-ended pin to its pad: a direct
// concurrent assign (buff:false) or one I/O-buffer instance. Differential pins
// (BufIBUFDS/BufOBUFDS) are handled by diffPairs and return (nil, nil) here.
func pinStmt(rp *elaborate.ResolvedPin) (vhdl.Stmt, error) {
	pin := &vhdl.Ident{Name: "pin_" + rp.Net}
	switch rp.BufferKind {
	case elaborate.BufDirect:
		if rp.PadDir == "in" { // pad drives the net: internal <= pin
			return concAssign(inExpr(rp), pin), nil
		}
		return concAssign(pin, outExpr(rp)), nil
	case elaborate.BufIBUF:
		return instBuf("ibuf_"+rp.Net, "IBUF",
			[]*vhdl.AssocElement{{Formal: "I", Actual: pin}, {Formal: "O", Actual: inExpr(rp)}},
			bufferGenerics(rp.Attrs, rp.BufferKind)), nil
	case elaborate.BufOBUF:
		return instBuf("obuf_"+rp.Net, "OBUF",
			[]*vhdl.AssocElement{{Formal: "I", Actual: outExpr(rp)}, {Formal: "O", Actual: pin}},
			bufferGenerics(rp.Attrs, rp.BufferKind)), nil
	case elaborate.BufOBUFT:
		return instBuf("obuft_"+rp.Net, "OBUFT",
			[]*vhdl.AssocElement{{Formal: "I", Actual: outExpr(rp)}, {Formal: "T", Actual: &vhdl.Ident{Name: rp.OutEn}}, {Formal: "O", Actual: pin}},
			bufferGenerics(rp.Attrs, rp.BufferKind)), nil
	case elaborate.BufIOBUF:
		return instBuf("iobuf_"+rp.Net, "IOBUF",
			[]*vhdl.AssocElement{{Formal: "I", Actual: outExpr(rp)}, {Formal: "T", Actual: &vhdl.Ident{Name: rp.OutEn}}, {Formal: "O", Actual: inExpr(rp)}, {Formal: "IO", Actual: pin}},
			bufferGenerics(rp.Attrs, rp.BufferKind)), nil
	default: // BufIBUFDS/BufOBUFDS handled elsewhere; unknown -> skip
		return nil, nil
	}
}

// instBuf builds a BARE component instantiation `<label> : <comp> generic map(..) port map(..)`
// (UnitKind 0 -> no entity/component keyword; prints `obuf_led0 : OBUF`).
func instBuf(label, comp string, ports, generics []*vhdl.AssocElement) *vhdl.InstantiationStmt {
	return &vhdl.InstantiationStmt{Label: label, Unit: comp, GenericMap: generics, PortMap: ports}
}
