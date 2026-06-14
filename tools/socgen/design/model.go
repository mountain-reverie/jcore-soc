package design

import (
	"fmt"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

type Design struct {
	Target             string                  `yaml:"target"`
	DeviceClasses      map[string]*DeviceClass `yaml:"device-classes"`
	Devices            []*Device               `yaml:"devices"`
	TopEntities        map[string]*TopEntity   `yaml:"top-entities"`
	PadringEntities    map[string]*TopEntity   `yaml:"padring-entities"`
	MergeSignals       map[string][]string     `yaml:"merge-signals"`
	ZeroSignals        []string                `yaml:"zero-signals"`
	BusWord            []string                `yaml:"bus-word"`
	BusWordLoopbackAck []string                `yaml:"bus-word-loopback-ack"`
	IRQ                map[string]*IRQEntry    `yaml:"irq"`
	Pins               *PinsSpec               `yaml:"pins"`
	PeripheralBuses    map[string]bool         `yaml:"peripheral-buses"`
	System             *System                 `yaml:"system"`
	Plugins            []string                `yaml:"plugins"`
}

// System holds the design `system:` block. Only the fields soc_gen consumes are
// modeled; other keys are ignored by the non-strict loader.
type System struct {
	DataBusDecode string   `yaml:"data-bus-decode"` // "simple" (default) | "exact"
	Pio           PioMap   `yaml:"pio"`             // PIO pi/po loopback spec (P5d-c)
	Dram          DramSpec `yaml:"dram"`            // [base, size] memory region (device tree)
}

// DramSpec is the [base, size] memory region (0x-hex or decimal). The zero value
// (both 0) means "unset" -> callers use DramOr's default.
type DramSpec [2]uint64

func (d *DramSpec) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind != yaml.SequenceNode || len(n.Content) != 2 {
		return &SpecError{Line: n.Line, Msg: "dram: expected a [base, size] pair"}
	}
	for i, e := range n.Content {
		v, err := strconv.ParseUint(e.Value, 0, 64) // base 0 -> auto 0x/decimal
		if err != nil {
			return &SpecError{Line: e.Line, Msg: fmt.Sprintf("dram[%d]: invalid uint %q", i, e.Value), Err: err}
		}
		d[i] = v
	}
	return nil
}

// DramOr returns the parsed [base, size] region, or the default
// [0x10000000, 0x8000000] when the dram key was absent (zero value).
func (s *System) DramOr() [2]uint64 {
	if s.Dram == (DramSpec{}) {
		return [2]uint64{0x10000000, 0x8000000}
	}
	return s.Dram
}

// PioEntry is one system.pio entry: a bit index or inclusive [Lo,Hi] range mapped
// to either an integer constant (Const) or a named loopback (Name). The general
// in/out/pin sub-cases are not yet modeled (mimas-unused); see P5d-c spec.
type PioEntry struct {
	Lo, Hi int
	Const  *int   // non-nil: drive pi(idx) with this constant value
	Name   string // map value's name (cosmetic; "" if a constant entry)
}

// PioMap parses the system.pio mapping, whose keys are an int index or a bracketed
// "[lo hi]" range and whose values are an int constant or a {name,...} map.
type PioMap []PioEntry

func (p *PioMap) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind != yaml.MappingNode {
		return &SpecError{Line: n.Line, Msg: fmt.Sprintf("pio: expected mapping, got node kind %d", n.Kind)}
	}
	out := make([]PioEntry, 0, len(n.Content)/2)
	for i := 0; i+1 < len(n.Content); i += 2 {
		kn, vn := n.Content[i], n.Content[i+1]
		lo, hi, err := parsePioKey(kn)
		if err != nil {
			return err
		}
		e := PioEntry{Lo: lo, Hi: hi}
		switch vn.Kind {
		case yaml.ScalarNode:
			c, err := strconv.Atoi(strings.TrimSpace(vn.Value))
			if err != nil {
				return &SpecError{Line: vn.Line, Msg: fmt.Sprintf("pio[%s]: expected int constant, got %q", kn.Value, vn.Value), Err: err}
			}
			if c != 0 && c != 1 {
				return &SpecError{Line: vn.Line, Msg: fmt.Sprintf("pio[%s]: constant must be 0 or 1 (a std_logic bit), got %d", kn.Value, c)}
			}
			e.Const = &c
		case yaml.MappingNode:
			var m struct {
				Name string `yaml:"name"`
			}
			if err := vn.Decode(&m); err != nil {
				return &SpecError{Line: vn.Line, Msg: fmt.Sprintf("pio[%s]: invalid value map", kn.Value), Err: err}
			}
			e.Name = m.Name
		default:
			return &SpecError{Line: vn.Line, Msg: fmt.Sprintf("pio[%s]: unexpected value node kind %d", kn.Value, vn.Kind)}
		}
		out = append(out, e)
	}
	*p = out
	return nil
}

// parsePioKey parses a pio key node: "[lo hi]" -> (lo,hi); a bare int "n" -> (n,n).
func parsePioKey(kn *yaml.Node) (lo, hi int, err error) {
	s := strings.TrimSpace(kn.Value)
	if strings.HasPrefix(s, "[") {
		if !strings.HasSuffix(s, "]") {
			return 0, 0, &SpecError{Line: kn.Line, Msg: fmt.Sprintf("pio key %q: want \"[lo hi]\"", s)}
		}
		inner := s[1 : len(s)-1]
		f := strings.Fields(inner)
		if len(f) != 2 {
			return 0, 0, &SpecError{Line: kn.Line, Msg: fmt.Sprintf("pio key %q: want \"[lo hi]\"", s)}
		}
		if lo, err = strconv.Atoi(f[0]); err != nil {
			return 0, 0, &SpecError{Line: kn.Line, Msg: fmt.Sprintf("pio key %q", s), Err: err}
		}
		if hi, err = strconv.Atoi(f[1]); err != nil {
			return 0, 0, &SpecError{Line: kn.Line, Msg: fmt.Sprintf("pio key %q", s), Err: err}
		}
		if lo > hi {
			return 0, 0, &SpecError{Line: kn.Line, Msg: fmt.Sprintf("pio key %q: lo > hi", s)}
		}
		return lo, hi, nil
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, 0, &SpecError{Line: kn.Line, Msg: fmt.Sprintf("pio key %q", s), Err: err}
	}
	return n, n, nil
}

// DtProp is one device-tree property, preserving its YAML source position.
type DtProp struct {
	Key string
	Val any
}

// DtProps is an ordered set of device-tree properties. The Clojure reference
// relies on EDN/YAML definition order for board.dts output, so this preserves
// source order rather than decoding into an unordered map.
type DtProps []DtProp

// UnmarshalYAML reads a mapping node's key/value pairs in source order, decoding
// each value generically into any (the same int/string/[]any/bool shapes that a
// map[string]any decode produced).
func (p *DtProps) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind != yaml.MappingNode {
		return &SpecError{Line: n.Line, Msg: "dt-props: expected a mapping"}
	}
	out := make(DtProps, 0, len(n.Content)/2)
	for i := 0; i+1 < len(n.Content); i += 2 {
		var v any
		if err := n.Content[i+1].Decode(&v); err != nil {
			return &SpecError{Line: n.Content[i+1].Line, Msg: "dt-props: value decode", Err: err}
		}
		out = append(out, DtProp{Key: n.Content[i].Value, Val: v})
	}
	*p = out
	return nil
}

// Get returns the value for key (and whether present). O(n); prop lists are tiny.
func (p DtProps) Get(key string) (any, bool) {
	for _, kv := range p {
		if kv.Key == key {
			return kv.Val, true
		}
	}
	return nil, false
}

// DtChild is one dt-children entry: a [name, {properties, children}] pair.
type DtChild struct {
	Name     string
	Props    DtProps
	Children []DtChild
}

// UnmarshalYAML decodes the 2-element [name, body] sequence; body is a mapping
// with an ordered "properties" map and an optional "children" sequence.
func (c *DtChild) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind != yaml.SequenceNode || len(n.Content) != 2 {
		return &SpecError{Line: n.Line, Msg: "dt-children entry: expected [name, body]"}
	}
	c.Name = n.Content[0].Value
	body := n.Content[1]
	if body.Kind != yaml.MappingNode {
		return &SpecError{Line: body.Line, Msg: "dt-children body: expected a mapping"}
	}
	for i := 0; i+1 < len(body.Content); i += 2 {
		key, val := body.Content[i].Value, body.Content[i+1]
		switch key {
		case "properties":
			if err := val.Decode(&c.Props); err != nil {
				return err
			}
		case "children":
			if err := val.Decode(&c.Children); err != nil {
				return err
			}
		}
	}
	return nil
}

type DeviceClass struct {
	Entity        string           `yaml:"entity"`
	Configuration string           `yaml:"configuration"`
	Architecture  string           `yaml:"architecture"`
	Desc          string           `yaml:"desc"`
	DtName        string           `yaml:"dt-name"`
	DtProps       DtProps          `yaml:"dt-props"`
	LeftAddrBit   int              `yaml:"left-addr-bit"`
	Regs          []*Reg           `yaml:"regs"`
	Generics      map[string]Value `yaml:"generics"`
	Ports         map[string]Value `yaml:"ports"`
	Requires      []string         `yaml:"requires"`
	DtChildren    []DtChild        `yaml:"dt-children"`
}

type Device struct {
	Class    string           `yaml:"class"`
	Name     string           `yaml:"name"`
	CPU      *int             `yaml:"cpu"`
	BaseAddr *Hex             `yaml:"base-addr"`
	IRQ      *IRQRef          `yaml:"irq"`
	Generics map[string]Value `yaml:"generics"`
	Ports    map[string]Value `yaml:"ports"`
	DtProps  DtProps          `yaml:"dt-props"`
	DtStdout bool             `yaml:"dt-stdout"`
	DtLabel  string           `yaml:"dt-label"`
	DtNode   *bool            `yaml:"dt-node"`
}

// IRQRef is a device interrupt reference. It is either a single IRQ line
// (Int set, e.g. `irq: 4`) or a set of named IRQ lines (Named set, e.g. the
// cache controller's `irq: {int0: {cpu: 0, irq: 3}, ...}`).
type IRQRef struct {
	Int   *int
	Named map[string]*IRQEntry
}

func (r *IRQRef) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind == yaml.MappingNode {
		m := map[string]*IRQEntry{}
		if err := n.Decode(&m); err != nil {
			return &SpecError{Line: n.Line, Msg: "invalid irq map", Err: err}
		}
		r.Named = m
		return nil
	}
	i, err := strconv.Atoi(n.Value)
	if err != nil {
		return &SpecError{Line: n.Line, Msg: fmt.Sprintf("invalid irq %q", n.Value), Err: err}
	}
	r.Int = &i
	return nil
}

// TopEntity is a top-level or padring entity instance. It names an entity
// directly (no device class); Entity defaults to the map key when blank. Set at
// most one of Architecture and Configuration — if both are given they must agree;
// if neither is given and the entity has a single architecture, that one is used.
type TopEntity struct {
	Entity        string           `yaml:"entity"`
	Configuration string           `yaml:"configuration"`
	Architecture  string           `yaml:"architecture"`
	Generics      map[string]Value `yaml:"generics"`
	Ports         map[string]Value `yaml:"ports"`
}

type Reg struct {
	Name  string `yaml:"name"`
	Addr  *int   `yaml:"addr"`
	Width *int   `yaml:"width"`
	Mode  string `yaml:"mode"`
	Type  string `yaml:"type"`
	Desc  string `yaml:"desc"`
}

type IRQEntry struct {
	CPU int   `yaml:"cpu"`
	IRQ int   `yaml:"irq"`
	DT  *bool `yaml:"dt?"`
}

// Hex is an unsigned integer that accepts 0x-prefixed or decimal YAML scalars.
type Hex uint64

func (h *Hex) UnmarshalYAML(n *yaml.Node) error {
	u, err := strconv.ParseUint(n.Value, 0, 64) // base 0 -> auto 0x/decimal
	if err != nil {
		return &SpecError{Line: n.Line, Msg: fmt.Sprintf("invalid address %q", n.Value), Err: err}
	}
	*h = Hex(u)
	return nil
}

// ValueKind tags how a generic/port value renders to VHDL.
type ValueKind int

const (
	KindExpr ValueKind = iota // verbatim VHDL identifier/expression (the default for plain scalars)
	KindStr                   // VHDL string literal (from !str)
	KindInt
	KindFloat
	KindBool
	KindMap // structured map value (e.g. an aic `irq_i: {irq?: true}`)
)

// Value is a generic/port value modeling verbatim-by-default: a plain YAML
// scalar is VHDL text (KindExpr); !str is a VHDL string literal; YAML
// numbers/bools are typed literals.
type Value struct {
	Kind  ValueKind
	Text  string // KindExpr (verbatim) or KindStr (literal text)
	Int   int64
	Float float64
	Bool  bool
	Map   map[string]any // KindMap (structured map value, preserved losslessly)
}

func (v *Value) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind == yaml.MappingNode {
		m := map[string]any{}
		if err := n.Decode(&m); err != nil {
			return &SpecError{Line: n.Line, Msg: "invalid value map", Err: err}
		}
		v.Kind, v.Map = KindMap, m
		return nil
	}
	if n.Kind != yaml.ScalarNode {
		return &SpecError{Line: n.Line, Msg: fmt.Sprintf("unsupported generic/port value kind %v", n.Kind)}
	}
	switch n.Tag {
	case "!str":
		v.Kind, v.Text = KindStr, n.Value
	case "!!int":
		i, err := strconv.ParseInt(n.Value, 0, 64)
		if err != nil {
			return &SpecError{Line: n.Line, Msg: fmt.Sprintf("invalid int %q", n.Value), Err: err}
		}
		v.Kind, v.Int = KindInt, i
	case "!!float":
		f, err := strconv.ParseFloat(n.Value, 64)
		if err != nil {
			return &SpecError{Line: n.Line, Msg: fmt.Sprintf("invalid float %q", n.Value), Err: err}
		}
		v.Kind, v.Float = KindFloat, f
	case "!!bool":
		b, err := strconv.ParseBool(n.Value)
		if err != nil {
			return &SpecError{Line: n.Line, Msg: fmt.Sprintf("invalid bool %q", n.Value), Err: err}
		}
		v.Kind, v.Bool = KindBool, b
	default: // "!!str" and any other plain scalar -> verbatim VHDL
		v.Kind, v.Text = KindExpr, n.Value
	}
	return nil
}

func (v Value) String() string {
	switch v.Kind {
	case KindStr:
		return strconv.Quote(v.Text)
	case KindInt:
		return strconv.FormatInt(v.Int, 10)
	case KindFloat:
		return strconv.FormatFloat(v.Float, 'g', -1, 64)
	case KindBool:
		return strconv.FormatBool(v.Bool)
	case KindMap:
		return fmt.Sprintf("%v", v.Map)
	default:
		return v.Text
	}
}
