package elaborate

import (
	"fmt"
	"sort"
)

// validateAddresses performs the elaborate-phase cross-device address checks:
// each memory-mapped device's base address is well-formed (Task 1), and no two
// device ranges overlap (Task 2). Best-effort; appends errors; never panics.
func validateAddresses(res *Resolution, errs []error) []error {
	if res == nil {
		return errs
	}
	for _, dev := range res.Devices {
		if dev.BaseAddr == nil {
			continue // not memory-mapped
		}
		base := *dev.BaseAddr
		rc := res.Classes[lc(dev.Class)]
		if rc == nil {
			continue // class unresolved — its mapping error already recorded
		}
		// bits[31:28] must be 0xA (the jcore memory-mapped device region)
		if base&0xF0000000 != 0xA0000000 {
			errs = append(errs, fmt.Errorf("device %q base address 0x%08x is invalid: bits 31-28 must be 0xA", dev.Name, base))
		}
		// Guard against an out-of-range LeftAddrBit (a bug upstream): a shift by >=64
		// yields 0, making the mask all-ones and flagging every address. Keep the
		// "best-effort, never misbehaves" contract explicit.
		if rc.LeftAddrBit < 0 || rc.LeftAddrBit >= 63 {
			errs = append(errs, fmt.Errorf("device %q class %q: left-addr-bit %d out of range [0,62]", dev.Name, dev.Class, rc.LeftAddrBit))
			continue
		}
		// The low leftAddrBit+1 bits are the device's internal address space and must
		// be zero in the base address (not over-specified). Faithful to soc_gen: the
		// check applies even when LeftAddrBit==0 (then the low 1 bit must be zero).
		mask := (uint64(1) << uint(rc.LeftAddrBit+1)) - 1
		if base&mask != 0 {
			errs = append(errs, fmt.Errorf("device %q base address 0x%08x has non-zero bits in its internal address range (low %d bits)", dev.Name, base, rc.LeftAddrBit+1))
		}
	}
	return checkAddrOverlap(res, errs)
}

// addrRange is a named [Lo,Hi] inclusive byte range.
type addrRange struct {
	Name   string
	Lo, Hi uint64
}

// reservedRegions are the hard-coded jcore memory regions that device ranges must
// not overlap (faithful to soc_gen's validate-devices).
var reservedRegions = []addrRange{
	{"sram", 0x00000000, 0x0FFFFFFF},
	{"dram", 0x10000000, 0x1FFFFFFF},
	{"cpumreg", 0xabcd0600, 0xabcd06FF},
}

// checkAddrOverlap reports every pair of overlapping address ranges among the
// memory-mapped devices and the reserved regions. Deterministic (sorted) output.
func checkAddrOverlap(res *Resolution, errs []error) []error {
	ranges := make([]addrRange, 0, len(res.Devices)+len(reservedRegions))
	for _, dev := range res.Devices {
		if dev.BaseAddr == nil {
			continue
		}
		rc := res.Classes[lc(dev.Class)]
		if rc == nil || rc.LeftAddrBit < 0 || rc.LeftAddrBit >= 63 {
			continue // unresolved class or out-of-range left-addr-bit (already errored)
		}
		base := *dev.BaseAddr
		span := uint64(1) << uint(rc.LeftAddrBit+1)
		ranges = append(ranges, addrRange{dev.Name, base, base + span - 1})
	}
	ranges = append(ranges, reservedRegions...)
	sort.Slice(ranges, func(i, j int) bool {
		if ranges[i].Lo != ranges[j].Lo {
			return ranges[i].Lo < ranges[j].Lo
		}
		return ranges[i].Name < ranges[j].Name
	})
	for i := 0; i < len(ranges); i++ {
		for j := i + 1; j < len(ranges); j++ {
			if ranges[i].Lo <= ranges[j].Hi && ranges[j].Lo <= ranges[i].Hi {
				errs = append(errs, fmt.Errorf("device memory regions overlap: %q [0x%08x,0x%08x] and %q [0x%08x,0x%08x]",
					ranges[i].Name, ranges[i].Lo, ranges[i].Hi, ranges[j].Name, ranges[j].Lo, ranges[j].Hi))
			}
		}
	}
	return errs
}
