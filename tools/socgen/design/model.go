package design

import (
	"fmt"
	"strconv"

	"gopkg.in/yaml.v3"
)

type Design struct {
	Target          string                  `yaml:"target"`
	DeviceClasses   map[string]*DeviceClass `yaml:"device-classes"`
	Devices         []*Device               `yaml:"devices"`
	TopEntities     map[string]*TopEntity   `yaml:"top-entities"`
	PadringEntities map[string]*TopEntity   `yaml:"padring-entities"`
	MergeSignals    map[string][]string     `yaml:"merge-signals"`
	ZeroSignals     []string                `yaml:"zero-signals"`
	IRQ             map[string]*IRQEntry    `yaml:"irq"`
}

type DeviceClass struct {
	Entity        string           `yaml:"entity"`
	Configuration string           `yaml:"configuration"`
	Architecture  string           `yaml:"architecture"`
	Desc          string           `yaml:"desc"`
	DtName        string           `yaml:"dt-name"`
	DtProps       map[string]any   `yaml:"dt-props"`
	LeftAddrBit   int              `yaml:"left-addr-bit"`
	Regs          []*Reg           `yaml:"regs"`
	Generics      map[string]Value `yaml:"generics"`
	Ports         map[string]Value `yaml:"ports"`
	Requires      []string         `yaml:"requires"`
	DtChildren    []any            `yaml:"dt-children"`
}

type Device struct {
	Class    string           `yaml:"class"`
	Name     string           `yaml:"name"`
	CPU      *int             `yaml:"cpu"`
	BaseAddr *Hex             `yaml:"base-addr"`
	IRQ      *IRQRef          `yaml:"irq"`
	Generics map[string]Value `yaml:"generics"`
	Ports    map[string]Value `yaml:"ports"`
	DtProps  map[string]any   `yaml:"dt-props"`
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
			return fmt.Errorf("line %d: invalid irq map: %w", n.Line, err)
		}
		r.Named = m
		return nil
	}
	i, err := strconv.Atoi(n.Value)
	if err != nil {
		return fmt.Errorf("line %d: invalid irq %q: %w", n.Line, n.Value, err)
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
		return fmt.Errorf("line %d: invalid address %q: %w", n.Line, n.Value, err)
	}
	*h = Hex(u)
	return nil
}

// ValueKind tags how a generic/port value renders to VHDL.
type ValueKind int

const (
	KindExpr  ValueKind = iota // verbatim VHDL identifier/expression (the default for plain scalars)
	KindStr                    // VHDL string literal (from !str)
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
			return fmt.Errorf("line %d: %w", n.Line, err)
		}
		v.Kind, v.Map = KindMap, m
		return nil
	}
	if n.Kind != yaml.ScalarNode {
		return fmt.Errorf("line %d: unsupported generic/port value kind %v", n.Line, n.Kind)
	}
	switch n.Tag {
	case "!str":
		v.Kind, v.Text = KindStr, n.Value
	case "!!int":
		i, err := strconv.ParseInt(n.Value, 0, 64)
		if err != nil {
			return fmt.Errorf("line %d: invalid int %q: %w", n.Line, n.Value, err)
		}
		v.Kind, v.Int = KindInt, i
	case "!!float":
		f, err := strconv.ParseFloat(n.Value, 64)
		if err != nil {
			return fmt.Errorf("line %d: invalid float %q: %w", n.Line, n.Value, err)
		}
		v.Kind, v.Float = KindFloat, f
	case "!!bool":
		b, err := strconv.ParseBool(n.Value)
		if err != nil {
			return fmt.Errorf("line %d: invalid bool %q: %w", n.Line, n.Value, err)
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
