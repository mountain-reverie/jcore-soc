package elaborate

import (
	"strings"
	"testing"
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
	errs := validateAddresses(res, nil)
	joined := ""
	for _, e := range errs {
		joined += e.Error() + "\n"
	}
	if !strings.Contains(joined, `device "badregion"`) || !strings.Contains(joined, "bits 31-28 must be 0xA") {
		t.Errorf("expected 0xA region error for badregion; got:\n%s", joined)
	}
	if !strings.Contains(joined, `device "overspec"`) || !strings.Contains(joined, "internal address range") {
		t.Errorf("expected over-specification error for overspec; got:\n%s", joined)
	}
	if strings.Contains(joined, `device "good"`) || strings.Contains(joined, `device "nomap"`) {
		t.Errorf("good/nomap must produce no error; got:\n%s", joined)
	}
}

func TestValidateBaseAddrBothChecksAppend(t *testing.T) {
	// 0xb0000001: wrong region (bits 31-28 != 0xA) AND over-specified (low bit set).
	// The two checks are independent and must BOTH append (best-effort, no short-circuit).
	res := &Resolution{
		Classes: map[string]*ResolvedClass{"c": {Name: "c", LeftAddrBit: 5}},
		Devices: []*ResolvedDevice{{Name: "bad", Class: "c", BaseAddr: u64(0xb0000001)}},
	}
	errs := validateAddresses(res, nil)
	region, overspec := false, false
	for _, e := range errs {
		if strings.Contains(e.Error(), "bits 31-28 must be 0xA") {
			region = true
		}
		if strings.Contains(e.Error(), "internal address range") {
			overspec = true
		}
	}
	if !region || !overspec {
		t.Errorf("expected BOTH region and over-spec errors; region=%v overspec=%v errs=%v", region, overspec, errs)
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
	errs := validateAddresses(res, nil)
	count := 0
	for _, e := range errs {
		if strings.Contains(e.Error(), "memory regions overlap") && strings.Contains(e.Error(), `"a"`) && strings.Contains(e.Error(), `"b"`) {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected exactly one a/b overlap error, got %d; errs: %v", count, errs)
	}
}

func TestValidateAddrOverlapReserved(t *testing.T) {
	// a device range overlapping the hard-coded cpumreg region [0xabcd0600,0xabcd06ff]
	res := &Resolution{
		Classes: map[string]*ResolvedClass{"c": {Name: "c", LeftAddrBit: 7}},
		Devices: []*ResolvedDevice{{Name: "clash", Class: "c", BaseAddr: u64(0xabcd0600)}},
	}
	errs := validateAddresses(res, nil)
	found := false
	for _, e := range errs {
		if strings.Contains(e.Error(), "memory regions overlap") && strings.Contains(e.Error(), "cpumreg") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected clash-vs-cpumreg overlap error; got %v", errs)
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
	for _, e := range validateAddresses(res, nil) {
		if strings.Contains(e.Error(), "overlap") {
			t.Errorf("unexpected overlap error: %v", e)
		}
	}
}
