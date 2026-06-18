package emit

import (
	"fmt"
	"strings"

	"github.com/j-core/jcore-soc/tools/socgen/elaborate"
)

// LPF renders a Lattice .lpf constraint file for an ECP5 target: a LOCATE +
// IOBUF line per padded pin. Per-pin IO standards and FREQUENCY constraints are
// added in Phase 2 from the translated pins section; Phase 1 defaults to
// LVCMOS33 so the build is wirable end to end.
func LPF(res *elaborate.Resolution) (string, error) {
	var b strings.Builder
	for _, p := range sortedPins(res) {
		if p.Pad == "" {
			continue
		}
		port := "pin_" + p.Net
		fmt.Fprintf(&b, "LOCATE COMP %q SITE %q;\n", port, p.Pad)
		fmt.Fprintf(&b, "IOBUF PORT %q IO_TYPE=LVCMOS33;\n", port)
	}
	return b.String(), nil
}
