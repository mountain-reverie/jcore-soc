package elaborate

import (
	"github.com/j-core/jcore-soc/tools/socgen/design"
	"github.com/j-core/jcore-soc/tools/socgen/iface"
)

type PortKind int

const (
	KindSignal  PortKind = iota // connects to GlobalSignal
	KindValue                   // tied to a constant Value
	KindIRQ                     // {irq?: ...} — recorded; routing is a later sub-milestone
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

// Resolution is the per-device resolution produced by Devices (P4b), plus the
// net-list and top/padring entities populated by Elaborate (P4c/P4d).
type Resolution struct {
	Classes         map[string]*ResolvedClass  // by class name (lower-cased key)
	Devices         []*ResolvedDevice          // spec order, unique names assigned
	TopEntities     map[string]*ResolvedEntity // by top-entity name (P4d)
	PadringEntities map[string]*ResolvedEntity // by padring-entity name (P4d)
	Signals         map[string]*Signal         // global net-list, populated by Elaborate
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

// ResolvedEntity is a resolved top-entity or padring-entity: an entity bound to
// an architecture/configuration with its ports built. Unlike a device it has no
// class — it names the entity directly (P4d).
type ResolvedEntity struct {
	Name     string
	Entity   *iface.Entity        // nil if the entity could not be bound
	ArchName string               // effective architecture name ("" if unresolved)
	Config   *iface.Configuration // non-nil iff resolved via a configuration
	Ports    []*ResolvedPort
}

// Signal is a global net that one or more ports (device, top/padring, or a
// synthetic driver) are connected to.
type Signal struct {
	Name  string
	Type  *ResolvedType
	Ports []*SignalPortRef
}

// SignalPortRef is a reference from a signal to one of its participating ports
// (device, top-entity, padring-entity, or synthetic driver).
type SignalPortRef struct {
	Context  Context
	PortName string
	Dir      string
	Type     *ResolvedType
}

// Context identifies the source of a SignalPortRef (device instance, top/padring
// entity, or synthetic driver).
type Context struct {
	Kind string // "device" | "zero" | "top" | "padring"
	ID   string
}
