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
			{Name: "overspec", Class: "c", BaseAddr: u64(0xabcd0001)},  // low 6 bits non-zero
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
