package design

import (
	"fmt"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// PinsSpec is the board "pins:" block: a reference to an external .pins file plus
// the regex/parametric rules that map pin names to net-list signals. Pins is
// populated by Load from File.
type PinsSpec struct {
	File  string     `yaml:"file"`
	Type  string     `yaml:"type"`
	Part  string     `yaml:"part"`
	Rules []*PinRule `yaml:"rules"`
	Pins  []*Pin     `yaml:"-"`
}

// Pin is one parsed entry from a .pins file: a net name and its FPGA pad.
type Pin struct {
	Net string
	Pad string
}

// PinRule maps matching pins to signals. A bare Signal infers direction; In/Out/
// OutEn name the tri-state legs directly. Attrs accumulate; Buff toggles I/O buffer.
type PinRule struct {
	Match  *Match           `yaml:"match"`
	Signal *SigSpec         `yaml:"signal"`
	In     *SigSpec         `yaml:"in"`
	Out    *SigSpec         `yaml:"out"`
	OutEn  *SigSpec         `yaml:"out-en"`
	Attrs  map[string]Value `yaml:"attrs"`
	Buff   *bool            `yaml:"buff"`
}

// SeqPart is one element of a parametric match or signal template: exactly one of
// Lit (a literal substring) or Sym (a numeric capture/substitution variable).
type SeqPart struct {
	Lit string
	Sym string
}

// Match is a pin-name matcher: either a Regex (scalar) or a parametric Parts
// sequence (literals + symbol captures, e.g. [mcb3_dram_a, n]).
type Match struct {
	Regex string
	Parts []SeqPart
}

// SigKind tags the target shape of a signal/in/out/out-en value.
type SigKind int

const (
	SigName     SigKind = iota // a literal signal name (may contain .elem or (idx))
	SigTrue                    // `true` -> use the pin's own net name
	SigConst                   // an integer constant (e.g. out: 0)
	SigTemplate                // a parts template (literals + symbols)
	SigMap                     // {name, diff: pos|neg}
)

// SigSpec is a signal target value in a pin rule.
type SigSpec struct {
	Kind  SigKind
	Name  string
	Int   int64
	Parts []SeqPart
	Diff  string
}

// isSymbolNode reports whether n is a parametric capture variable.
// Convention: a bare (unquoted, Style==0) single ASCII letter in a sequence is a
// symbol (e.g. `n` in `[mcb3_dram_a, n]`); anything quoted, multi-character, or
// non-alpha is a literal substring. The Style==0 check is what lets a quoted
// `"n"` be a literal, and the single-letter+alpha check is what separates `n`
// from `_` or a digit.
func isSymbolNode(n *yaml.Node) bool {
	if n.Kind != yaml.ScalarNode || n.Style != 0 || len(n.Value) != 1 {
		return false
	}
	c := n.Value[0]
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func seqParts(n *yaml.Node) []SeqPart {
	parts := make([]SeqPart, 0, len(n.Content))
	for _, e := range n.Content {
		if isSymbolNode(e) {
			parts = append(parts, SeqPart{Sym: e.Value})
		} else {
			parts = append(parts, SeqPart{Lit: e.Value})
		}
	}
	return parts
}

func (m *Match) UnmarshalYAML(n *yaml.Node) error {
	switch n.Kind {
	case yaml.ScalarNode:
		m.Regex = n.Value
	case yaml.SequenceNode:
		m.Parts = seqParts(n)
	default:
		return fmt.Errorf("line %d: invalid match node", n.Line)
	}
	return nil
}

func (s *SigSpec) UnmarshalYAML(n *yaml.Node) error {
	switch n.Kind {
	case yaml.ScalarNode:
		switch n.Tag {
		case "!!bool":
			if n.Value != "true" { // only `signal: true` is meaningful (use the pin's own net name)
				return fmt.Errorf("line %d: signal bool must be true, got %q", n.Line, n.Value)
			}
			s.Kind = SigTrue
		case "!!int":
			i, err := strconv.ParseInt(n.Value, 0, 64)
			if err != nil {
				return fmt.Errorf("line %d: invalid int signal %q: %w", n.Line, n.Value, err)
			}
			s.Kind, s.Int = SigConst, i
		default:
			s.Kind, s.Name = SigName, n.Value
		}
	case yaml.SequenceNode:
		s.Kind, s.Parts = SigTemplate, seqParts(n)
	case yaml.MappingNode:
		s.Kind = SigMap
		var m struct {
			Name string `yaml:"name"`
			Diff string `yaml:"diff"`
		}
		if err := n.Decode(&m); err != nil {
			return fmt.Errorf("line %d: invalid signal map: %w", n.Line, err)
		}
		s.Name, s.Diff = m.Name, m.Diff
	default:
		return fmt.Errorf("line %d: invalid signal node", n.Line)
	}
	return nil
}

// parsePinNames parses the simple "NAME PAD" .pins format (one pin per line; '#'
// comments and blank lines skipped; net lower-cased). The pad is optional (a
// pad-less net yields Pad==""), matching the Clojure parser; any extra fields are
// ignored. The []error result is for symmetry with parsePinList; this parser has
// no error conditions today.
func parsePinNames(data []byte) ([]*Pin, []error) {
	var pins []*Pin
	var errs []error
	for line := range strings.SplitSeq(string(data), "\n") {
		t := strings.TrimSpace(line)
		if t == "" || strings.HasPrefix(t, "#") {
			continue
		}
		f := strings.Fields(t)
		p := &Pin{Net: strings.ToLower(f[0])}
		if len(f) > 1 {
			p.Pad = f[1]
		}
		pins = append(pins, p)
	}
	return pins, errs
}

// parsePinList parses the EAGLE columnar .pins export (columns: Part Pad Pin Dir
// Net). Lines before the `part` row are skipped; parsing stops at the next blank
// line; '*** unconnected ***' and '#' lines are dropped; net is normalized
// (lower-case, '-'->'_', '!' removed).
func parsePinList(data []byte, part string) ([]*Pin, []error) {
	var pins []*Pin
	var errs []error
	lines := strings.Split(string(data), "\n")
	i := 0
	for i < len(lines) && !strings.HasPrefix(lines[i], part) {
		i++
	}
	if i == len(lines) {
		return nil, []error{fmt.Errorf("pin-list: part %q not found", part)}
	}
	lines[i] = strings.TrimPrefix(lines[i], part) // strip the part token from the first row
	for ; i < len(lines); i++ {
		raw := lines[i]
		t := strings.TrimSpace(raw)
		if t == "" {
			break
		}
		if strings.HasPrefix(t, "#") || strings.Contains(raw, "*** unconnected ***") {
			continue
		}
		f := strings.Fields(raw)
		if len(f) < 4 {
			continue // not a pin row
		}
		net := strings.ToLower(f[3])
		net = strings.ReplaceAll(net, "-", "_")
		net = strings.ReplaceAll(net, "!", "")
		pins = append(pins, &Pin{Net: net, Pad: f[0]})
	}
	return pins, errs
}
