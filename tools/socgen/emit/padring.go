package emit

import (
	"errors"
	"sort"
	"strconv"
	"strings"

	"github.com/j-core/jcore-soc/tools/socgen/design"
	"github.com/j-core/jcore-soc/tools/socgen/elaborate"
	"github.com/j-core/jcore-soc/tools/socgen/vhdl"
)

// bufferGenericAttrs are the attributes that belong to the I/O buffer instances
// (P5d-b), not the pad port; they are excluded from pad-port attributes.
// Must stay in sync with bufferGenericOrder (iobufs.go), which emits this same
// set as the buffer generic map.
var bufferGenericAttrs = map[string]bool{"iostandard": true, "drive": true, "slew": true, "diff_term": true}

// sortedPins returns res.Pins sorted by net for deterministic emission.
func sortedPins(res *elaborate.Resolution) []*elaborate.ResolvedPin {
	ps := append([]*elaborate.ResolvedPin(nil), res.Pins...)
	sort.Slice(ps, func(i, j int) bool { return natLess(ps[i].Net, ps[j].Net) })
	return ps
}

// natLess reports whether net a sorts before net b, replicating the original
// soc_gen `:sort` key (devices.clj): the key is
//
//	[firstWord, inner]
//
// where firstWord is the leading non-digit run, and inner is [firstWord, num, net]
// when the net has a first digit run (num = its integer value) or [net] when the
// net has no digits. Keys compare firstWord first, then inner element-by-element
// (Clojure vector order: a shorter vector that is a prefix of the other is less).
// This sorts e.g. "lpddr_a9" < "lpddr_a10" (first number differs) while keeping
// "mcb3_dram_a10" < "mcb3_dram_a2" (first number "3" ties, so the full net breaks
// the tie bytewise) — matching both the microboard and mimas golden orderings.
func natLess(a, b string) bool {
	fa, na, ia, hasA := pinSortKey(a)
	fb, nb, ib, hasB := pinSortKey(b)
	if fa != fb {
		return fa < fb
	}
	// inner vectors: [firstWord, num, net] (digits) or [firstWord] (no digits).
	// Element 0 is firstWord, already equal. Element 1 is num (only present when
	// the net has digits); a vector without it is the shorter prefix → less.
	if hasA != hasB {
		// Shorter inner vector ([firstWord]) is a Clojure-prefix → less. So a<b
		// iff a has no digits (its key is the prefix of b's [firstWord, num, net]).
		return !hasA
	}
	if !hasA { // neither has digits; inner is [firstWord] == [firstWord]
		return false
	}
	if na != nb {
		return na < nb
	}
	return ia < ib // element 2: the full net, bytewise
}

// pinSortKey returns the original soc_gen sort components for a net: firstWord (the
// leading non-digit run), the integer value of the first digit run and the full net
// (when present), and whether the net contains a digit run.
func pinSortKey(s string) (firstWord string, num int64, net string, hasDigit bool) {
	i := 0
	for i < len(s) && (s[i] < '0' || s[i] > '9') {
		i++
	}
	firstWord = s[:i]
	if i >= len(s) {
		return firstWord, 0, s, false
	}
	j := i
	for j < len(s) && s[j] >= '0' && s[j] <= '9' {
		j++
	}
	n, _ := strconv.ParseInt(s[i:j], 10, 64)
	return firstWord, n, s, true
}

// attrText renders a pad-attribute value as its VHDL string content: a bool as
// lowercase "true"/"false", an int as its digits, otherwise the verbatim text.
func attrText(v design.Value) string {
	switch v.Kind {
	case design.KindBool:
		return strconv.FormatBool(v.Bool)
	case design.KindInt:
		return strconv.FormatInt(v.Int, 10)
	default: // KindExpr / KindStr — verbatim text; the caller wraps it in VHDL quotes
		return v.Text
	}
}

// padRingPorts builds the pad_ring entity ports: one `pin_<net> : <dir> std_logic`
// per resolved pin (both legs of a differential pair are separate pins).
func padRingPorts(res *elaborate.Resolution) []*vhdl.InterfaceDecl {
	pins := sortedPins(res)
	out := make([]*vhdl.InterfaceDecl, 0, len(pins))
	for _, p := range pins {
		dir := p.PadDir
		if dir == "" {
			dir = "in"
		}
		out = append(out, &vhdl.InterfaceDecl{Names: []string{"pin_" + p.Net}, Mode: dir, SubtypeMark: stdLogicMark})
	}
	return out
}

// pinAttrs builds the pad-port attribute declarations + specifications: `loc` (the
// pad) plus any non-buffer-generic attribute (e.g. tig). Buffer generics
// (iostandard/drive/slew/diff_term) belong to the I/O buffers (P5d-b).
func pinAttrs(res *elaborate.Resolution) []vhdl.Decl {
	pins := sortedPins(res)
	// collect distinct attribute names for the decls. `loc` is declared first
	// (only when at least one pin carries a pad), then the other attribute
	// names in first-seen/sorted order.
	var otherDecls []string
	hasLoc := false
	seen := map[string]bool{}
	type spec struct{ name, ent, val string }
	var specs []spec
	for _, p := range pins {
		port := "pin_" + p.Net
		if p.Pad != "" {
			hasLoc = true
			specs = append(specs, spec{"loc", port, vhdlEscape(p.Pad)})
		}
		// other attrs (sorted) excluding buffer generics and loc.
		var keys []string
		for k := range p.Attrs {
			lk := lc(k)
			if lk == "loc" || bufferGenericAttrs[lk] {
				continue
			}
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			lk := lc(k)
			if !seen[lk] {
				seen[lk] = true
				otherDecls = append(otherDecls, lk)
			}
			specs = append(specs, spec{lk, port, vhdlEscape(attrText(p.Attrs[k]))})
		}
	}
	declOrder := otherDecls
	if hasLoc {
		declOrder = append([]string{"loc"}, otherDecls...)
	}
	out := make([]vhdl.Decl, 0, len(declOrder)+len(specs))
	for _, name := range declOrder {
		out = append(out, &vhdl.AttributeDecl{Name: name, TypeMark: "string"})
	}
	// `-- Pin attributes` leads the per-pin specifications (after the bare
	// `attribute <name> : string;` declarations); omitted when there are none.
	if len(specs) > 0 {
		out = append(out, &vhdl.Comment{Text: "Pin attributes"})
	}
	for _, s := range specs {
		out = append(out, &vhdl.AttributeSpec{
			Name: s.name, Entities: []string{s.ent}, EntityClass: vhdl.SIGNAL,
			Value: &vhdl.BasicLit{Kind: vhdl.STRINGLIT, Value: `"` + s.val + `"`},
		})
	}
	return out
}

// invertSig is an inverted-pad intermediate: the generated signal name and its
// source signal, e.g. {name: "pad_reset_n", src: "reset"} for eth_rst's
// out: {name: reset, invert: true}.
type invertSig struct{ name, src string }

// invertSignalName returns the intermediate signal name for an inverted out: leg
// — "pad_<base>_n" (Clojure generate.clj update-pin-signals). A bus/record
// element ref collapses its '.'/'(' separators to '_' (rare; none in the repo).
func invertSignalName(ref string) string {
	if strings.ContainsAny(ref, ".(") {
		return "pad_" + strings.NewReplacer(".", "_", "(", "_", ")", "").Replace(ref) + "_n"
	}
	return "pad_" + ref + "_n"
}

// invertIntermediates collects the deduped inverted-out intermediates, in
// sorted-pin (= Clojure invert-signals insertion) order.
func invertIntermediates(res *elaborate.Resolution) []invertSig {
	seen := map[string]bool{}
	out := make([]invertSig, 0, len(res.Pins))
	for _, p := range sortedPins(res) {
		if !p.OutInvert || p.Out == "" {
			continue
		}
		name := invertSignalName(p.Out)
		if seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, invertSig{name: name, src: p.Out})
	}
	return out
}

// invertAssigns emits the `pad_<base>_n <= not <base>;` concurrent assignments for
// the inverted-out intermediates (one per distinct intermediate).
func invertAssigns(res *elaborate.Resolution) []vhdl.Stmt {
	ints := invertIntermediates(res)
	out := make([]vhdl.Stmt, 0, len(ints))
	for _, iv := range ints {
		out = append(out, concAssign(&vhdl.Ident{Name: iv.name},
			&vhdl.UnaryExpr{Op: vhdl.NOT, X: &vhdl.Ident{Name: iv.src}}))
	}
	return out
}

// socInstStmt instantiates the soc architecture inside pad_ring, wiring each
// PadringTop port to the same-named pad_ring signal (name => name).
func socInstStmt(res *elaborate.Resolution) *vhdl.InstantiationStmt {
	inst := &vhdl.InstantiationStmt{Label: "soc", UnitKind: vhdl.ENTITY, Unit: "work.soc", Arch: "impl"}
	if res.SignalLocations != nil {
		for _, pl := range res.SignalLocations.PadringTop {
			inst.PortMap = append(inst.PortMap, &vhdl.AssocElement{Formal: pl.Name, Actual: &vhdl.Ident{Name: pl.Name}})
		}
	}
	return inst
}

// PadRing renders pad_ring.vhd: the FPGA top entity (pin_<net> ports + LOC/TIG
// attributes) and an architecture instantiating soc + the padring entities,
// declaring the Padring internal signals, and wiring the pads — the soc instance,
// padring entities, the system.pio pi/po loopback (P5d-c), then the I/O buffers +
// direct-wires (P5d-b). Best-effort; never panics.
func PadRing(res *elaborate.Resolution) (string, error) {
	if res == nil {
		return "", nil
	}
	var errs []error

	decls := pinAttrs(res)
	if res.SignalLocations != nil {
		names := append([]string(nil), res.SignalLocations.Padring...)
		sort.Strings(names)
		for _, n := range names {
			var typ *elaborate.ResolvedType
			if s := res.Signals[n]; s != nil {
				typ = s.Type
			}
			mark, con := typeToSubtype(typ)
			decls = append(decls, &vhdl.SignalDecl{Names: []string{n}, SubtypeMark: mark, Constraint: con})
		}
	}
	// Inverted-pad intermediates (e.g. pad_reset_n) are appended after the sorted
	// Padring signals, matching golden (generate.clj appends `vals invert-signals`).
	for _, iv := range invertIntermediates(res) {
		decls = append(decls, &vhdl.SignalDecl{Names: []string{iv.name}, SubtypeMark: stdLogicMark})
	}

	stmts := []vhdl.Stmt{socInstStmt(res)}
	for _, name := range sortedTopNames(res.PadringEntities) {
		re := res.PadringEntities[name]
		if re.Entity == nil && re.Config == nil {
			// An unbound padring-entity (no VHDL source for the entity) is dropped
			// best-effort. KNOWN DIVERGENCE (microboard): `eth_clk_bufs` is declared a
			// padring-entity but has no entity source in the repo, so we (and the current
			// Clojure) cannot bind it — its golden instance + the eth_rx_clk/eth_tx_clk
			// boundary ports it would drive are a stale artifact (same class as the
			// clock_locked1/rtc divergences). Consequently eth_rx_clk/eth_tx_clk are
			// emitted as undriven internal signals, not boundary ports. See
			// TestPadRingMicroboardFormatting (which excises the eth pads).
			errs = append(errs, &EmitError{Kind: ErrUnboundEntity, Inst: re.Name})
			continue
		}
		stmts = append(stmts, topInstStmt(re))
	}

	stmts = append(stmts, pioStatements(res)...)
	// Inverted-pad assignments (pad_reset_n <= not reset) sit between the pio
	// statements and the pin buffers/direct-wires, matching golden.
	stmts = append(stmts, invertAssigns(res)...)

	pinStmts, perr := pinStatements(res)
	if perr != nil {
		errs = append(errs, perr)
	}
	stmts = append(stmts, pinStmts...)

	ctx := padringContext(res)

	df := &vhdl.DesignFile{
		Context: ctx,
		Units: []vhdl.DesignUnit{
			&vhdl.EntityDecl{Name: "pad_ring", Ports: padRingPorts(res)},
			&vhdl.ArchitectureBody{Name: "impl", Entity: "pad_ring", Decls: decls, Stmts: stmts},
		},
	}
	sortInstMaps(df)
	return withBanner(vhdl.Print(df)), errors.Join(errs...)
}
