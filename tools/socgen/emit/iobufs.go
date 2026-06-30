package emit

import (
	"errors"
	"fmt"
	"sort"
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
	if rp.OutConst != "" {
		return &vhdl.BasicLit{Kind: vhdl.CHARLIT, Value: rp.OutConst}
	}
	if rp.OutInvert {
		// the pad is driven by the inverted-source intermediate (e.g. pad_reset_n);
		// PadRing declares it and assigns pad_reset_n <= not reset.
		return &vhdl.Ident{Name: invertSignalName(rp.Out)}
	}
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
		// A real buff:false pin always has a leg or Signal; the empty-both guards
		// below only skip degenerate pins that would emit `pin_x <= ;`.
		if rp.PadDir == "in" { // pad drives the net: internal <= pin
			if rp.In == "" && rp.Signal == "" { // nothing to drive; skip
				return nil, nil
			}
			return concAssign(inExpr(rp), pin), nil
		}
		// PadDir "out" (and "") drives the pad; BufDirect is never "inout".
		if rp.Out == "" && rp.Signal == "" { // nothing drives the pad; skip
			return nil, nil
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

// signalBase is the base signal name of a ref (the leading run up to the first
// '.' or '('), used to group differential pairs by their shared signal.
func signalBase(ref string) string {
	if i := strings.IndexAny(ref, ".("); i >= 0 {
		return ref[:i]
	}
	return ref
}

// pinStatements builds the architecture statements wiring every pad: one direct
// assign or buffer per single-ended pin (in sortedPins net order), then the
// differential-pair buffers. Best-effort; accumulates per-pin errors.
func pinStatements(res *elaborate.Resolution) ([]vhdl.Stmt, error) {
	if externalConstraints(res.Target) {
		return ecp5PinStatements(res)
	}
	pins := sortedPins(res)
	var stmts []vhdl.Stmt
	var diffs []*elaborate.ResolvedPin
	var errs []error
	for _, rp := range pins {
		if rp.BufferKind == elaborate.BufIBUFDS || rp.BufferKind == elaborate.BufOBUFDS {
			diffs = append(diffs, rp)
			continue
		}
		st, err := pinStmt(rp)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if st != nil {
			stmts = append(stmts, st)
		}
	}
	dstmts, derr := diffPairs(diffs)
	if derr != nil {
		errs = append(errs, derr)
	}
	stmts = append(stmts, dstmts...)
	return stmts, errors.Join(errs...)
}

// diffPairs groups differential pins by their shared signal base and emits one
// IBUFDS/OBUFDS per complete pos/neg pair. A pair missing a leg -> ErrDiffPair.
func diffPairs(diffs []*elaborate.ResolvedPin) ([]vhdl.Stmt, error) {
	groups := map[string][]*elaborate.ResolvedPin{}
	var order []string
	for _, rp := range diffs {
		b := signalBase(rp.Signal)
		if _, ok := groups[b]; !ok {
			order = append(order, b)
		}
		groups[b] = append(groups[b], rp)
	}
	sort.Strings(order)
	stmts := make([]vhdl.Stmt, 0, len(order))
	var errs []error
	for _, b := range order {
		// elaborate guarantees at most one pos and one neg per shared signal.
		var pos, neg *elaborate.ResolvedPin
		for _, rp := range groups[b] {
			switch rp.Diff {
			case "pos":
				pos = rp
			case "neg":
				neg = rp
			}
		}
		if pos == nil || neg == nil {
			name := b
			if pos != nil {
				name = pos.Net
			} else if neg != nil {
				name = neg.Net
			}
			errs = append(errs, &EmitError{Kind: ErrDiffPair, Inst: name})
			continue
		}
		comp, prefix := "OBUFDS", "obufds_"
		posPin := &vhdl.Ident{Name: "pin_" + pos.Net}
		negPin := &vhdl.Ident{Name: "pin_" + neg.Net}
		var ports []*vhdl.AssocElement
		// both legs always carry the same BufferKind (one rule resolves the pair).
		if pos.BufferKind == elaborate.BufIBUFDS {
			comp, prefix = "IBUFDS", "ibufds_"
			ports = []*vhdl.AssocElement{{Formal: "I", Actual: posPin}, {Formal: "IB", Actual: negPin}, {Formal: "O", Actual: inExpr(pos)}}
		} else {
			ports = []*vhdl.AssocElement{{Formal: "I", Actual: outExpr(pos)}, {Formal: "O", Actual: posPin}, {Formal: "OB", Actual: negPin}}
		}
		stmts = append(stmts, instBuf(prefix+pos.Net+"_"+neg.Net, comp, ports, bufferGenerics(pos.Attrs, pos.BufferKind)))
	}
	return stmts, errors.Join(errs...)
}

// instBuf builds a BARE component instantiation `<label> : <comp> generic map(..) port map(..)`
// (UnitKind 0 -> no entity/component keyword; prints `obuf_led0 : OBUF`).
func instBuf(label, comp string, ports, generics []*vhdl.AssocElement) *vhdl.InstantiationStmt {
	return &vhdl.InstantiationStmt{Label: label, Unit: comp, GenericMap: generics, PortMap: ports}
}

// ecp5PinStatements wires each pad straight to its internal net with a direct
// concurrent assignment — no vendor IO buffer. The ECP5 yosys/nextpnr flow
// infers IO from plain top-level ports, so pad_ring needs no UNISIM primitives.
// Inout/differential pins are deferred to Phase 2 (handled via padring entities
// such as sdram_iocells) and surface here as an explicit error.
func ecp5PinStatements(res *elaborate.Resolution) ([]vhdl.Stmt, error) {
	var stmts []vhdl.Stmt
	var errs []error
	for _, rp := range sortedPins(res) {
		sts, err := ecp5PinStmt(rp)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		stmts = append(stmts, sts...)
	}
	return stmts, errors.Join(errs...)
}

// ecp5PinStmt emits the direct concurrent assignment(s) that wire one single-ended
// pad to its internal net (no vendor buffer). The device leg handles In/Out/OutConst
// following the Xilinx path logic. The signal leg, when rp.Signal != "", wires the
// named internal signal directly: for input pads (PadDir "in"), `signal <= pad`;
// for output pads, `pad <= signal`. Pin shapes whose ECP5 handling is deferred to
// Phase 2 — differential, inout/tristate (OutEn), bidirectional (both legs), and
// inverted outputs (which need the inverted-intermediate machinery PadRing builds
// only for Xilinx) — return an explicit error rather than emitting wrong hardware.
func ecp5PinStmt(rp *elaborate.ResolvedPin) ([]vhdl.Stmt, error) {
	switch {
	case rp.BufferKind == elaborate.BufEntity:
		return nil, nil // pad is owned by a padring-entity; emit nothing here
	case rp.Diff != "":
		return nil, fmt.Errorf("ecp5 pin %q: differential pads are deferred to Phase 2", rp.Net)
	case rp.OutEn != "":
		return nil, fmt.Errorf("ecp5 pin %q: inout/tristate pads are deferred to Phase 2", rp.Net)
	case rp.In != "" && rp.Out != "":
		return nil, fmt.Errorf("ecp5 pin %q: bidirectional pads are deferred to Phase 2", rp.Net)
	case rp.Out != "" && rp.Signal != "":
		return nil, fmt.Errorf("ecp5 pin %q: output pad with Signal would create two drivers on the pad", rp.Net)
	}
	pin := &vhdl.Ident{Name: "pin_" + rp.Net}
	var stmts []vhdl.Stmt
	// device leg
	switch {
	case rp.OutInvert && rp.Out != "":
		stmts = append(stmts, concAssign(pin, &vhdl.UnaryExpr{Op: vhdl.NOT, X: &vhdl.Ident{Name: rp.Out}}))
	case rp.In != "": // input pad: internal <= pin
		stmts = append(stmts, concAssign(inExpr(rp), pin))
	case rp.Out != "" || rp.OutConst != "": // output pad: pin <= internal (incl. constant)
		stmts = append(stmts, concAssign(pin, outExpr(rp)))
	}
	// signal leg: a pad whose rule has a `signal:` target wires that internal
	// signal too (in addition to any device leg). Direction follows PadDir.
	// We use &vhdl.Ident{Name: rp.Signal} directly — identical to what the old
	// bare-signal path produced via inExpr/outExpr (which fall through to
	// rp.Signal when In/Out are empty). Complex refs like "sd_cmd.a(0)" are
	// rendered as-is by vhdl.Ident, matching the old path byte-for-byte.
	if rp.Signal != "" {
		sig := &vhdl.Ident{Name: rp.Signal}
		if rp.PadDir == "in" {
			stmts = append(stmts, concAssign(sig, pin)) // signal <= pad
		} else {
			stmts = append(stmts, concAssign(pin, sig)) // pad <= signal
		}
	}
	if len(stmts) == 0 {
		return nil, fmt.Errorf("ecp5 pin %q: unsupported pin shape (no leg or signal)", rp.Net)
	}
	return stmts, nil
}
