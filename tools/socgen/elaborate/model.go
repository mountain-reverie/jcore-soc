package elaborate

import (
	"github.com/j-core/jcore-soc/tools/socgen/design"
	"github.com/j-core/jcore-soc/tools/socgen/iface"
)

type PortKind int

const (
	KindSignal  PortKind = iota // connects to GlobalSignal
	KindValue                   // tied to a constant Value
	KindIRQ                     // {irq?: ...} — recorded; routing is P4d
	KindDataBus                 // {data-bus: ...} — recorded; bus mux is P5
	KindDeferred                // an unsupported map kind (bist/ring/open) — recorded only
)

type ResolvedPort struct {
	Name         string
	Dir          string        // from the entity port: "in"|"out"|"inout"|"buffer"|""
	Type         *ResolvedType
	Kind         PortKind
	GlobalSignal string        // Kind==KindSignal
	Value        *design.Value // Kind==KindValue
}

// Resolution is the per-device resolution produced by Devices (P4b).
type Resolution struct {
	Classes map[string]*ResolvedClass // by class name (lower-cased key)
	Devices []*ResolvedDevice         // spec order, unique names assigned
}

type ResolvedClass struct {
	Name        string
	Entity      *iface.Entity        // nil if the entity could not be bound
	ArchName    string               // effective architecture name ("" if unresolved)
	Config      *iface.Configuration // non-nil iff resolved via a configuration
	Regs        []*ResolvedReg       // T2
	LeftAddrBit int                  // T2 (0 if no registers)
	RegRange    [2]int               // T2
	Generics    map[string]design.Value
}

type ResolvedReg struct {
	Name      string
	Addr      int
	Width     int
	ByteRange [2]int
	Mode      string
	Type      string
}

type ResolvedDevice struct {
	Name     string
	Class    string
	Generics map[string]design.Value // effective (class overlaid by instance)
	BaseAddr *uint64                 // carried, validated in P4e
	Ports    []*ResolvedPort
}
