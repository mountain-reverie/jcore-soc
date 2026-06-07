package elaborate

import "fmt"

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
		// the low leftAddrBit+1 bits are the device's internal address space and
		// must be zero in the base address (not over-specified)
		mask := (uint64(1) << uint(rc.LeftAddrBit+1)) - 1
		if base&mask != 0 {
			errs = append(errs, fmt.Errorf("device %q base address 0x%08x has non-zero bits in its internal address range (low %d bits)", dev.Name, base, rc.LeftAddrBit+1))
		}
	}
	return errs
}
