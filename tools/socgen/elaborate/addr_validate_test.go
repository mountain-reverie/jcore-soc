package elaborate

import (
	"errors"
	"testing"

	"github.com/j-core/jcore-soc/tools/socgen/internal/errutil"
)

func u64(v uint64) *uint64 { return &v }

func TestValidateBaseAddr(t *testing.T) {
	res := &Resolution{
		Classes: map[string]*ResolvedClass{
			"c": {Name: "c", LeftAddrBit: 5}, // internal span = 2^6 = 64 bytes -> low 6 bits must be 0
		},
		Devices: []*ResolvedDevice{
			{Name: "good", Class: "c", BaseAddr: u64(0xabcd0000)},      // valid
			{Name: "badregion", Class: "c", BaseAddr: u64(0xb0000000)}, // bits 31-28 != 0xA
			{Name: "overspec", Class: "c", BaseAddr: u64(0xabcd0001)},  // bit 0 set (within the low 6 bits that must be zero)
			{Name: "nomap", Class: "c"},                                // BaseAddr nil -> ignored
		},
	}
	err := validateAddresses(res)
	if !addrErrFor(err, "badregion", ErrBadRegion) {
		t.Errorf("expected 0xA region error for badregion; got: %v", err)
	}
	if !addrErrFor(err, "overspec", ErrOverSpec) {
		t.Errorf("expected over-specification error for overspec; got: %v", err)
	}
	// good/nomap must produce no base-address error (region or over-spec). A
	// good/overspec range overlap is a separate, legitimate error and is ignored.
	for _, e := range errutil.Errors(err) {
		var ae *AddrError
		if errors.As(e, &ae) && (ae.Device == "good" || ae.Device == "nomap") &&
			(errors.Is(ae, ErrBadRegion) || errors.Is(ae, ErrOverSpec)) {
			t.Errorf("good/nomap must produce no base-address error; got: %v", e)
		}
	}
}

func TestValidateBaseAddrBothChecksAppend(t *testing.T) {
	// 0xb0000001: wrong region (bits 31-28 != 0xA) AND over-specified (low bit set).
	// The two checks are independent and must BOTH append (best-effort, no short-circuit).
	res := &Resolution{
		Classes: map[string]*ResolvedClass{"c": {Name: "c", LeftAddrBit: 5}},
		Devices: []*ResolvedDevice{{Name: "bad", Class: "c", BaseAddr: u64(0xb0000001)}},
	}
	err := validateAddresses(res)
	if !errors.Is(err, ErrBadRegion) || !errors.Is(err, ErrOverSpec) {
		t.Errorf("expected BOTH region and over-spec errors; err=%v", err)
	}
}

func TestValidateAddrOverlap(t *testing.T) {
	res := &Resolution{
		Classes: map[string]*ResolvedClass{"c": {Name: "c", LeftAddrBit: 7}}, // span 2^8 = 256
		Devices: []*ResolvedDevice{
			{Name: "a", Class: "c", BaseAddr: u64(0xabcd0000)}, // [0xabcd0000, 0xabcd00ff]
			{Name: "b", Class: "c", BaseAddr: u64(0xabcd0080)}, // overlaps a
		},
	}
	err := validateAddresses(res)
	count := 0
	for _, e := range errutil.Errors(err) {
		var ae *AddrError
		if errors.As(e, &ae) && errors.Is(ae, ErrAddrOverlap) &&
			((ae.Device == "a" && ae.Other == "b") || (ae.Device == "b" && ae.Other == "a")) {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected exactly one a/b overlap error, got %d; err: %v", count, err)
	}
}

func TestValidateAddrOverlapReserved(t *testing.T) {
	// a device range overlapping the hard-coded cpumreg region [0xabcd0600,0xabcd06ff]
	res := &Resolution{
		Classes: map[string]*ResolvedClass{"c": {Name: "c", LeftAddrBit: 7}},
		Devices: []*ResolvedDevice{{Name: "clash", Class: "c", BaseAddr: u64(0xabcd0600)}},
	}
	err := validateAddresses(res)
	found := false
	for _, e := range errutil.Errors(err) {
		var ae *AddrError
		if errors.As(e, &ae) && errors.Is(ae, ErrAddrOverlap) &&
			(ae.Device == "cpumreg" || ae.Other == "cpumreg") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected clash-vs-cpumreg overlap error; got %v", err)
	}
}

func TestValidateAddrNoOverlap(t *testing.T) {
	// two valid, non-overlapping devices, well above the sram [0,0x0FFFFFFF] /
	// dram [0x10000000,0x1FFFFFFF] reserved windows: a=[0xabcd0000,0xabcd003f],
	// b=[0xabcd0040,0xabcd007f], both clear of cpumreg [0xabcd0600,0xabcd06ff].
	res := &Resolution{
		Classes: map[string]*ResolvedClass{"c": {Name: "c", LeftAddrBit: 5}}, // span 64
		Devices: []*ResolvedDevice{
			{Name: "a", Class: "c", BaseAddr: u64(0xabcd0000)},
			{Name: "b", Class: "c", BaseAddr: u64(0xabcd0040)},
		},
	}
	if err := validateAddresses(res); errors.Is(err, ErrAddrOverlap) {
		t.Errorf("unexpected overlap error: %v", err)
	}
}

// addrErrFor reports whether err carries an AddrError of kind for the named device.
func addrErrFor(err error, device string, kind error) bool {
	for _, e := range errutil.Errors(err) {
		var ae *AddrError
		if errors.As(e, &ae) && errors.Is(ae, kind) && ae.Device == device {
			return true
		}
	}
	return false
}
