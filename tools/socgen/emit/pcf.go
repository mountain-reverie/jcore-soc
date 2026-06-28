package emit

import (
	"fmt"
	"strings"

	"github.com/j-core/jcore-soc/tools/socgen/elaborate"
)

// PCF renders a nextpnr-ice40 .pcf for an iCE40 target: one `set_io` line per
// padded pin. iCE40 board I/O is fixed LVCMOS33, so no IO standard is emitted
// here (mirrors emit.LPF, which is the ECP5 analog).
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
