package elaborate

import (
	"errors"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/j-core/jcore-soc/tools/socgen/design"
)

// validatePinInvert rejects invert:true on any leg other than out:. Only the
// out: leg invert is implemented (faithful to the only repo usage, turtle
// eth_rst); a signal:/in:/out-en: invert would otherwise be silently dropped and
// emit wrong VHDL, so it is surfaced as a loud elaboration error instead.
func validatePinInvert(d *design.Design) error {
	if d == nil || d.Pins == nil {
		return nil
	}
	var errs []error
	for _, r := range d.Pins.Rules {
		check := func(leg string, s *design.SigSpec) {
			if s != nil && s.Invert {
				errs = append(errs, fmt.Errorf("%w (on the %s leg)", ErrUnsupportedInvert, leg))
			}
		}
		check("signal", r.Signal)
		check("in", r.In)
		check("out-en", r.OutEn)
	}
	return errors.Join(errs...)
}

// matchPin returns the captured symbol->int environment if rule matches pin net.
// A regex match is full-anchored; a parametric match builds a regex from the
// rule's Parts (literals escaped, symbols -> ([0-9]+) captures).
func matchPin(rule *design.PinRule, net string) (map[string]int, bool) {
	m := rule.Match
	if m == nil {
		return nil, false
	}
	if len(m.Parts) == 0 {
		re, err := regexp.Compile("^(?:" + m.Regex + ")$")
		if err != nil {
			return nil, false
		}
		return map[string]int{}, re.MatchString(net)
	}
	pat := "^"
	var syms []string
	for _, p := range m.Parts {
		if p.Sym != "" {
			pat += "([0-9]+)"
			syms = append(syms, p.Sym)
		} else {
			pat += regexp.QuoteMeta(p.Lit)
		}
	}
	pat += "$"
	re, err := regexp.Compile(pat)
	if err != nil {
		return nil, false
	}
	g := re.FindStringSubmatch(net)
	if g == nil {
		return nil, false
	}
	env := map[string]int{}
	for i, s := range syms {
		env[s], _ = strconv.Atoi(g[i+1])
	}
	return env, true
}

// expandSig resolves a SigSpec to a concrete signal-ref string given the match
// env and the pin's net. The returned kind is SigConst for constant targets,
// else SigName (the ref is a fully-resolved name; sub-signal splitting is later).
func expandSig(s *design.SigSpec, pinNet string, env map[string]int) (ref, diff string, kind design.SigKind) {
	if s == nil {
		return "", "", design.SigName
	}
	switch s.Kind {
	case design.SigTrue:
		return pinNet, "", design.SigName
	case design.SigConst:
		return "", "", design.SigConst
	case design.SigMap:
		return s.Name, s.Diff, design.SigName
	case design.SigTemplate:
		out := ""
		for _, p := range s.Parts {
			if p.Sym != "" {
				out += strconv.Itoa(env[p.Sym])
			} else {
				out += p.Lit
			}
		}
		return out, "", design.SigName
	default: // SigName
		return s.Name, "", design.SigName
	}
}

// folded is the accumulated effect of all rules matching one pin.
type folded struct {
	attrs      map[string]design.Value
	buff       *bool
	signalRef  string // bare signal: (direction auto-inferred later)
	signalDiff string
	inRef      string
	outRef     string
	outEnRef   string
	outConst   *int64 // integer value of a constant out: leg (nil if not const)
	outInvert  bool   // out: leg has invert: true (drive the pad with the inverted source)
}

// foldRules applies every matching rule to pin in order: attrs accumulate, and
// signal/in/out/out-en/buff take the last matching rule's value (expanded with
// that rule's capture env).
func foldRules(rules []*design.PinRule, pin *design.Pin) folded {
	f := folded{attrs: map[string]design.Value{}}
	for _, r := range rules {
		env, ok := matchPin(r, pin.Net)
		if !ok {
			continue
		}
		maps.Copy(f.attrs, r.Attrs)
		if r.Buff != nil {
			f.buff = r.Buff
		}
		if r.Signal != nil {
			ref, diff, _ := expandSig(r.Signal, pin.Net, env)
			f.signalRef, f.signalDiff = ref, diff
		}
		if r.In != nil {
			ref, _, _ := expandSig(r.In, pin.Net, env)
			f.inRef = ref
		}
		if r.Out != nil {
			ref, _, kind := expandSig(r.Out, pin.Net, env)
			f.outRef = ref
			f.outInvert = r.Out.Invert
			if kind == design.SigConst {
				v := r.Out.Int
				f.outConst = &v
			} else {
				f.outConst = nil
			}
		}
		if r.OutEn != nil {
			ref, _, _ := expandSig(r.OutEn, pin.Net, env)
			f.outEnRef = ref
		}
	}
	return f
}

// splitSignal splits a signal ref into its base signal name and, when the ref
// targets a bus/record element, the full element ref (else ""). The base is the
// leading run up to the first '.' or '('.
func splitSignal(ref string) (base, element string) {
	i := strings.IndexAny(ref, ".(")
	if i < 0 {
		return ref, ""
	}
	return ref[:i], ref
}

// bareSignalDir infers the net-list direction of a bare-`signal:` pin port: if
// the target signal already has a NON-pin driver (device/top/padring), the pin
// consumes it ("in"); otherwise the pin drives it ("out"). Absent signal -> the
// pin drives. Pin-context ports are skipped so that one member of a differential
// pair (or one of several pins on the same bare signal) does not flip the
// inferred direction of the others — all members must agree on direction.
func bareSignalDir(sigs map[string]*Signal, base string) string {
	s := sigs[base]
	if s == nil {
		return dirOut
	}
	for _, p := range s.Ports {
		if p.Context.Kind == ctxKindPin {
			continue
		}
		if isDriver(p.Dir) {
			return dirIn
		}
	}
	return dirOut
}

// signalIsReal reports whether base names a real net at pin-resolution time: a
// signal with at least one non-pin port (device/top/padring/synthetic driver),
// or a declared zero-signal. A signal that exists only via pin legs is not real
// — faithful to the Clojure global-signals membership test (devices.clj).
func signalIsReal(sigs map[string]*Signal, d *design.Design, base string) bool {
	if s := sigs[base]; s != nil {
		for _, p := range s.Ports {
			if p.Context.Kind != ctxKindPin {
				return true
			}
		}
	}
	return slices.Contains(d.ZeroSignals, base)
}

// resolvePins folds the rules over each pin, joins the resulting legs into the
// net-list (sigs), and returns the resolved pins (with buffer kind + attrs). It
// must run AFTER device/top/padring gather so a bare-`signal:` pin's direction can
// be inferred from existing drivers, and BEFORE zero-signals.
func resolvePins(d *design.Design, sigs map[string]*Signal) []*ResolvedPin {
	if d == nil || d.Pins == nil {
		return nil
	}
	out := make([]*ResolvedPin, 0, len(d.Pins.Pins))
	for _, pin := range d.Pins.Pins {
		f := foldRules(d.Pins.Rules, pin)
		// Constant-driven output pin (e.g. atmel_rst out: 1, eth_mdc out: 0): the
		// out: leg targets an integer constant, not a signal, so it adds no net-list
		// leg. Emit it as an OBUF whose I is a '0'/'1' literal (Clojure :out <int>).
		// A non-0/1 constant is out of scope and dropped. (A constant out-en, which
		// would belong on an OBUFT T input, does not occur in any repo board and is
		// not handled here.)
		if f.outConst != nil {
			n := *f.outConst
			if n != 0 && n != 1 {
				continue
			}
			lit := "'0'"
			if n == 1 {
				lit = "'1'"
			}
			out = append(out, &ResolvedPin{
				Net: pin.Net, Pad: pin.Pad, Attrs: f.attrs,
				BufferKind: BufOBUF, PadDir: dirOut, OutConst: lit,
			})
			continue
		}
		// Skip pins with no signal connection (matched only attr rules, or nothing):
		// these are unmapped pads, ignored — faithful to the Clojure :ignore.
		// (Constant-driven pins are handled above and never reach this point.)
		if f.signalRef == "" && f.inRef == "" && f.outRef == "" && f.outEnRef == "" {
			continue
		}
		// Drop a pin if any of its signal legs targets a non-real net (no
		// device/top/padring port and not a zero-signal) — faithful to Clojure
		// :missing (devices.clj match-pins-to-signals): a pin with any missing leg
		// is filtered out. mimas io_p* pads map to io_p<n> signals no device
		// declares; turtle's eth_intr/sd_det/usb_clk/vid_en target signals no
		// device uses. Both are dropped. (Constant pins have no signal leg and are
		// handled elsewhere, so they are never dropped here.)
		drop := false
		for _, ref := range []string{f.signalRef, f.inRef, f.outRef, f.outEnRef} {
			if ref == "" {
				continue
			}
			if base, _ := splitSignal(ref); !signalIsReal(sigs, d, base) {
				drop = true
				break
			}
		}
		if drop {
			continue
		}
		rp := &ResolvedPin{Net: pin.Net, Pad: pin.Pad, Attrs: f.attrs, Diff: f.signalDiff}
		bareDir := ""
		if f.signalRef != "" {
			base, elem := splitSignal(f.signalRef)
			rp.Signal = f.signalRef
			bareDir = bareSignalDir(sigs, base)
			addPinPort(sigs, pin.Net, "signal", base, elem, bareDir, f.signalDiff)
		}
		// in: -> driver (out); out:/out-en: -> consumer (in)
		if f.inRef != "" {
			base, elem := splitSignal(f.inRef)
			rp.In = f.inRef
			addPinPort(sigs, pin.Net, "in", base, elem, dirOut, "")
		}
		if f.outRef != "" {
			base, elem := splitSignal(f.outRef)
			rp.Out = f.outRef
			rp.OutInvert = f.outInvert
			addPinPort(sigs, pin.Net, "out", base, elem, dirIn, "")
		}
		if f.outEnRef != "" {
			base, elem := splitSignal(f.outEnRef)
			rp.OutEn = f.outEnRef
			addPinPort(sigs, pin.Net, "out-en", base, elem, dirIn, "")
		}
		rp.BufferKind = bufferKind(f, bareDir)
		rp.PadDir = padDir(rp.BufferKind, bareDir)
		out = append(out, rp)
	}
	return out
}

// padDir is the pad's physical direction. Buffered pins follow their buffer kind;
// a BufDirect (direct-wire) pad takes the opposite of the bare-signal direction
// (the pin drives the net => input pad; consumes it => output pad).
// A BufDirect pin with an empty bareDir (explicit in/out/out-en legs + buff:false)
// falls through to dirOut here and is refined in P5d-b.
func padDir(bk BufferKind, bareDir string) string {
	switch bk {
	case BufIBUF, BufIBUFDS:
		return dirIn
	case BufOBUF, BufOBUFT, BufOBUFDS:
		return dirOut
	case BufIOBUF:
		return dirInout
	default: // BufDirect
		if bareDir == dirOut {
			return dirIn
		}
		return dirOut
	}
}

// addPinPort joins one pin leg to the net-list under its base signal name.
func addPinPort(sigs map[string]*Signal, net, leg, base, element, dir, diff string) {
	if base == "" {
		return
	}
	s := sigs[base]
	if s == nil {
		s = &Signal{Name: base}
		sigs[base] = s
	}
	s.Ports = append(s.Ports, &SignalPortRef{
		Context:  Context{Kind: ctxKindPin, ID: net},
		PortName: "pin." + net + "." + leg,
		Dir:      dir,
		Type:     s.Type,
		Element:  element,
		Diff:     diff,
	})
}

// bufferKind selects the semantic I/O buffer from the folded pin shape. bareDir is
// the auto-inferred net-list direction of a bare-`signal:` pin ("out" = the pin
// drives the net, so the pad is an INPUT; "in" = the pin consumes, pad is OUTPUT);
// it is "" for pins using explicit in/out/out-en legs.
func bufferKind(f folded, bareDir string) BufferKind {
	if f.buff != nil && !*f.buff {
		return BufDirect
	}
	in, out, outEn := f.inRef != "", f.outRef != "", f.outEnRef != ""
	if f.signalRef != "" && !in && !out && !outEn {
		// a bare single-ended (or differential) signal pin: input pad if it drives
		// the net, output pad if it consumes it.
		switch {
		case f.signalDiff != "" && bareDir == dirOut:
			return BufIBUFDS
		case f.signalDiff != "":
			return BufOBUFDS
		case bareDir == dirOut:
			return BufIBUF
		default:
			return BufOBUF
		}
	}
	switch {
	case in && out && outEn:
		return BufIOBUF
	case out && outEn:
		return BufOBUFT
	case out:
		return BufOBUF
	case in:
		return BufIBUF
	default:
		return BufIBUF
	}
}
