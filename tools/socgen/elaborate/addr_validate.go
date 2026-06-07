package elaborate

import (
	"fmt"
	"sort"
)

// maxLeftAddrBit is the largest LeftAddrBit we can compute a span for without the
// `1 << (LeftAddrBit+1)` shift reaching/exceeding uint64's width.
const maxLeftAddrBit = 62

// validateAddresses performs the elaborate-phase cross-device address checks:
// each memory-mapped device's base address is well-formed (0xA region +
// over-specification), and no two device/reserved ranges overlap. Best-effort:
// unlike the Clojure soc_gen (which gates later checks behind `when-not error?`),
// every independent check runs and ALL issues are surfaced in one pass; a device
// may therefore yield more than one error. Appends errors; never panics.
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
		if rc.LeftAddrBit < 0 || rc.LeftAddrBit > maxLeftAddrBit {
			errs = append(errs, fmt.Errorf("device %q class %q: left-addr-bit %d out of range [0,%d]", dev.Name, dev.Class, rc.LeftAddrBit, maxLeftAddrBit))
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

// deviceSpan returns a memory-mapped device's inclusive [lo,hi] byte range, or
// ok=false to skip it (not memory-mapped, unresolved class, or an out-of-range
// LeftAddrBit already flagged by validateAddresses). hi saturates at MaxUint64 so
// a base near the top of the address space cannot wrap below lo.
func deviceSpan(dev *ResolvedDevice, rc *ResolvedClass) (lo, hi uint64, ok bool) {
	if dev.BaseAddr == nil || rc == nil || rc.LeftAddrBit < 0 || rc.LeftAddrBit > maxLeftAddrBit {
		return 0, 0, false
	}
	lo = *dev.BaseAddr
	hi = lo + (uint64(1) << uint(rc.LeftAddrBit+1)) - 1
	if hi < lo { // wraparound near the top of the address space
		hi = ^uint64(0)
	}
	return lo, hi, true
}

// checkAddrOverlap reports every pair of overlapping address ranges among the
// memory-mapped devices and the reserved regions. Deterministic (sorted) output.
// Precondition: res != nil (validateAddresses, the only caller, guarantees it).
func checkAddrOverlap(res *Resolution, errs []error) []error {
	ranges := make([]addrRange, 0, len(res.Devices)+len(reservedRegions))
	for _, dev := range res.Devices {
		lo, hi, ok := deviceSpan(dev, res.Classes[lc(dev.Class)])
		if !ok {
			continue
		}
		ranges = append(ranges, addrRange{dev.Name, lo, hi})
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
				errs = append(errs, fmt.Errorf("memory regions overlap: %q [0x%08x,0x%08x] and %q [0x%08x,0x%08x]",
					ranges[i].Name, ranges[i].Lo, ranges[i].Hi, ranges[j].Name, ranges[j].Lo, ranges[j].Hi))
			}
		}
	}
	return errs
}
