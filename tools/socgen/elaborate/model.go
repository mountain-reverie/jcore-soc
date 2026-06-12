package elaborate

import (
	"github.com/j-core/jcore-soc/tools/socgen/design"
	"github.com/j-core/jcore-soc/tools/socgen/iface"
)

type PortKind int

const (
	KindSignal   PortKind = iota // connects to GlobalSignal
	KindValue                    // tied to a constant Value
	KindIRQ                      // {irq?: ...} — recorded; routing is a later sub-milestone
	KindDataBus                  // cpu data-bus port (set by classifyDataBus); wired to devs_bus_i/o by P5b
	KindDeferred                 // an unsupported map kind (bist/ring/open) — recorded only
)

// Port directions (entity port modes) and context kinds, factored out of the
// repeated string literals they label.
const (
	dirIn     = "in"
	dirOut    = "out"
	dirInout  = "inout"
	dirBuffer = "buffer"

	ctxKindPin = "pin"

	dirDownto = "downto"
)

type ResolvedPort struct {
	Name         string
	Dir          string // from the entity port: "in"|"out"|"inout"|"buffer"|""
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
	Pins            []*ResolvedPin             // resolved pins (P4d-ii)
	DataBus         *PeripheralBusModel        // P5b; nil if no data-bus devices
	SignalLocations *SignalLocations           // P5c-i
	Pio             []PioBit                   // P5d-c: resolved system.pio loopback bits (sorted by Idx)
	Library         *iface.Library             // the bound interface library (for emit type introspection, P5c-ii-b)
	IRQ             *IRQModel                  // P5e: AIC1 interrupt wiring (nil if no aic)
}

// IRQModel is the resolved AIC1 interrupt wiring (P5e), faithful to irq.clj.
type IRQModel struct {
	Signals       []IRQSignal                  // irqs<cpu> (Width 8) + OR-source signals (Width 0), in this order
	OrAssigns     []IRQOrAssign                // irqs<cpu>(path) <= a or b or …
	PortOverrides map[string]map[string]string // devName -> portName -> actual text ("irqs0(4)"|"open"|"irq_0_5_a"|"irqs0")
	VectorNumbers map[string][8]int            // aicDevName -> vector value per path (0 unused)
}

// IRQSignal is one IRQ-related signal declaration.
type IRQSignal struct {
	Name  string
	Width int // 8 => std_logic_vector(7 downto 0) := (others=>'0'); 0 => std_logic (OR source)
}

// IRQOrAssign is one OR-combine: Target <= Sources[0] or Sources[1] or …
type IRQOrAssign struct {
	Target  string
	Sources []string
}

// PioBit is one resolved system.pio bit: a loopback (pi(Idx) <= po(Idx)) when
// Const is nil, else a constant tie (pi(Idx) <= Const).
type PioBit struct {
	Idx   int
	Const *int
	Name  string // pio group name (e.g. "led"); "" for a constant entry
}

// PortLoc is a boundary signal that becomes an entity port (P5c).
type PortLoc struct{ Name, Dir string }

// SignalLocations partitions the net-list into the soc/devices boundary ports and
// the per-entity internal signals (a port of Clojure categorize-signals). P5c-ii
// emits from this.
type SignalLocations struct {
	PadringTop   []PortLoc         // soc.vhd entity ports (sorted by Name)
	TopDevices   []PortLoc         // devices.vhd entity ports (sorted by Name)
	Padring      []string          // padring-internal (sorted)
	Top          []string          // soc-arch internal (sorted)
	Devices      []string          // devices-arch internal (sorted)
	TopExtra     map[string]string // name -> "sig_"+name (output-port also read in top)
	DevicesExtra map[string]string // name -> "sig_"+name (driven & read across devices)
}

// PeripheralBusModel is the resolved data-bus master topology (P5b).
type PeripheralBusModel struct {
	MasterBus    string      // final master bus name (e.g. "cpu0" or "cpu01")
	Connected    []string    // connected master bus names (sorted)
	Disconnected []string    // disconnected master bus names (sorted) -> loopback
	MuxStages    []*MuxStage // arbitration mux chain (empty for a single master)
	DecodeMode   string      // "simple" | "exact"
}

// MuxStage is one multi-master arbitration mux instantiation.
type MuxStage struct {
	Label  string // instance label, e.g. "cpus_mux"
	Entity string // mux entity, e.g. "multi_master_bus_mux"
	In1    string // first master bus name
	In2    string // second master bus name
	Out    string // produced bus name, e.g. "cpu01"
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
	DataBus  bool // entity carries a cpu data-bus port pair (set by classifyDataBus)
	// GenericTypes maps lower-cased entity generic name → resolved type, for typed
	// value rendering (e.g. a std_logic_vector generic constant → sized hex literal).
	GenericTypes map[string]*ResolvedType
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
	Generics map[string]design.Value // top/padring-entity generics (P6b-3a)
}

// BufferKind is the semantic I/O buffer a pin needs; emit (P5) instantiates it.
type BufferKind int

const (
	BufDirect BufferKind = iota // buff:false — direct wire, no buffer
	BufIBUF
	BufOBUF
	BufOBUFT
	BufIOBUF
	BufIBUFDS
	BufOBUFDS
)

// ResolvedPin is a board pin resolved to its signal refs, buffer kind, and attrs.
// The actual buffer/constraint VHDL is emitted in P5. BufferKind==BufDirect is the
// single source of truth for "no I/O buffer" (a buff:false rule).
type ResolvedPin struct {
	Net, Pad   string
	Signal     string // bare-signal ref ("" if in/out/out-en used)
	In         string
	Out        string
	OutEn      string
	Diff       string
	BufferKind BufferKind
	PadDir     string // the pad's physical direction: "in"|"out"|"inout" (P5d)
	Attrs      map[string]design.Value
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
	Element  string // the full element ref (e.g. "dr_data_o.dqo(0)") iff a pin targets a bus/record element; "" if whole-signal
	Diff     string // "pos"|"neg"|"" for differential pin pairs
}

// Context identifies the source of a SignalPortRef (device instance, top/padring
// entity, or synthetic driver).
type Context struct {
	Kind string // "device" | "zero" | "top" | "padring" | "pin"
	ID   string
}
