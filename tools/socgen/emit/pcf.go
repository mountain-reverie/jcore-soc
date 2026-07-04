package emit

import (
	"fmt"
	"strings"

	"github.com/j-core/jcore-soc/tools/socgen/elaborate"
)

// PCF renders a nextpnr-ice40 .pcf for an iCE40 target: one `set_io` line per
// padded pin. iCE40 board I/O is fixed LVCMOS33, so no IO standard is emitted
// here (mirrors emit.LPF, which is the ECP5 analog).
//
// NOTE on SB_LVDS_INPUT (mdi1_p): nextpnr-ice40's PCF grammar is only
// `set_io [-nowarn] [-pullup ...] [-pullup_resistor ...] cell pin` -- it has
// NO io-standard directive (an `-io_std` token makes nextpnr treat the pin as
// unconstrained and abort). The iCE40 LVDS differential-input standard is
// selected instead by the `IO_STANDARD => "SB_LVDS_INPUT"` parameter on the
// pin's SB_IO cell in the netlist, which lives in the (socgen-generated)
// pad_ring. So the `iostandard: SB_LVDS_INPUT` attribute on the mdi1_p pin in
// design.yaml is meaningful only to the ECP5 LPF path; for iCE40 it is a
// documented bring-up step (see targets/boards/icesugar/README.md), NOT a PCF
// line -- emitting it here would break place-and-route.
func PCF(res *elaborate.Resolution) (string, error) {
	var b strings.Builder
	for _, p := range sortedPins(res) {
		if p.Pad == "" {
			continue
		}
		fmt.Fprintf(&b, "set_io pin_%s %s\n", p.Net, p.Pad)
	}
	return b.String(), nil
}
