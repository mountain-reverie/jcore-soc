package elaborate

import (
	"errors"
	"fmt"
)

var (
	ErrEntityNotFound     = errors.New("unable to map to entity")
	ErrConfigNotFound     = errors.New("configuration not found")
	ErrArchNotFound       = errors.New("architecture not found")
	ErrArchConfigMismatch = errors.New("architecture and configuration mismatch")
	ErrNoArch             = errors.New("no architecture for entity")
	ErrAmbiguousArch      = errors.New("ambiguous architecture for entity")
	ErrNoLibrary          = errors.New("no library")
	ErrUnknownClass       = errors.New("unknown class")
	ErrUnknownGeneric     = errors.New("unknown generic")
	ErrDuplicateName      = errors.New("duplicate device name")
	ErrRegisterOverlap    = errors.New("register overlap")
	// ErrUnsupportedInvert: a pin rule sets invert:true on a leg other than out:.
	// Only the out: leg invert is implemented (the only case any repo board uses);
	// other legs would be silently mis-emitted, so they are rejected loudly.
	ErrUnsupportedInvert = errors.New("invert: true is only supported on an out: pin leg")
	// ErrLeftAddrBitTooSmall: a class's configured left-addr-bit can't cover its
	// registers (resolveRegs, design-time). Distinct from ErrLeftAddrBit, the
	// elaboration-time range guard in validateAddresses — so errors.Is can tell
	// the two apart.
	ErrLeftAddrBitTooSmall = errors.New("left-addr-bit too small for registers")
	ErrLeftAddrBit         = errors.New("left-addr-bit out of range")
	ErrDataBusPorts        = errors.New("malformed data-bus port pair")
)

// ResolveError reports a device/class/entity resolution failure. Ctx names the
// subject (e.g. `class "spi"` / `device "flash"`). Detail carries the formatted
// specifics where the message is richer than Name alone; when Detail is set it is
// the whole message and Name does NOT appear in Error() — but Name remains
// populated for errors.As consumers, so callers must not assume the two are
// redundant.
type ResolveError struct {
	Kind   error
	Ctx    string
	Name   string
	Detail string
}

func (e *ResolveError) Error() string {
	if e.Detail != "" {
		return fmt.Sprintf("%s: %s", e.Ctx, e.Detail)
	}
	return fmt.Sprintf("%s: %v %q", e.Ctx, e.Kind, e.Name)
}
func (e *ResolveError) Unwrap() error { return e.Kind }

var (
	ErrTypeMismatch       = errors.New("type mismatch")
	ErrMultiDriver        = errors.New("signal driven by multiple ports")
	ErrUndrivenSignal     = errors.New("nothing drives signal")
	ErrMultiContextDriver = errors.New("signal driven from multiple contexts")
)

// SignalError reports a net-list validation failure for a global signal.
type SignalError struct {
	Kind   error
	Signal string
	Detail string // joined port list / type list
}

func (e *SignalError) Error() string {
	switch {
	case errors.Is(e.Kind, ErrTypeMismatch):
		return fmt.Sprintf("type mismatch for signal %q: %s", e.Signal, e.Detail)
	case errors.Is(e.Kind, ErrMultiDriver):
		return fmt.Sprintf("signal %q is driven by multiple ports: %s", e.Signal, e.Detail)
	case errors.Is(e.Kind, ErrUndrivenSignal):
		return fmt.Sprintf("nothing drives signal %q used by %s", e.Signal, e.Detail)
	case errors.Is(e.Kind, ErrMultiContextDriver):
		return fmt.Sprintf("signal %q driven from multiple contexts: %s", e.Signal, e.Detail)
	default:
		return fmt.Sprintf("signal %q: %v", e.Signal, e.Kind)
	}
}
func (e *SignalError) Unwrap() error { return e.Kind }

var (
	ErrBadRegion   = errors.New("base address bits 31-28 must be 0xA")
	ErrOverSpec    = errors.New("base address over-specified")
	ErrAddrOverlap = errors.New("memory regions overlap")
)

// AddrError reports an address-validation failure.
type AddrError struct {
	Kind             error
	Device           string
	Class            string // ErrLeftAddrBit: the device's class
	Base             uint64
	Bits             int    // ErrOverSpec: number of low bits that must be zero
	Other            string // overlap: the other region's name
	Lo, Hi           uint64 // overlap: this region's range
	OLo, OHi         uint64 // overlap: the other region's range
	LeftAddrBit, Max int    // ErrLeftAddrBit
}

func (e *AddrError) Error() string {
	switch {
	case errors.Is(e.Kind, ErrBadRegion):
		return fmt.Sprintf("device %q base address 0x%08x is invalid: bits 31-28 must be 0xA", e.Device, e.Base)
	case errors.Is(e.Kind, ErrOverSpec):
		return fmt.Sprintf("device %q base address 0x%08x has non-zero bits in its internal address range (low %d bits)", e.Device, e.Base, e.Bits)
	case errors.Is(e.Kind, ErrAddrOverlap):
		return fmt.Sprintf("memory regions overlap: %q [0x%08x,0x%08x] and %q [0x%08x,0x%08x]", e.Device, e.Lo, e.Hi, e.Other, e.OLo, e.OHi)
	case errors.Is(e.Kind, ErrLeftAddrBit):
		return fmt.Sprintf("device %q class %q: left-addr-bit %d out of range [0,%d]", e.Device, e.Class, e.LeftAddrBit, e.Max)
	default:
		return fmt.Sprintf("device %q: %v", e.Device, e.Kind)
	}
}
func (e *AddrError) Unwrap() error { return e.Kind }
