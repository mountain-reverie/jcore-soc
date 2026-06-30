package emit

import (
	"fmt"
	"strings"

	"github.com/j-core/jcore-soc/tools/socgen/design"
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
		// nextpnr-ecp5's package pin database is case-sensitive and uses
		// UPPER-case site names (e.g. "D20"); the .pins parser lower-cases the
		// pad, so re-upper it here or nextpnr rejects the constraint as a
		// non-existent pin.
		fmt.Fprintf(&b, "LOCATE COMP %q SITE %q;\n", port, strings.ToUpper(p.Pad))
		fmt.Fprintf(&b, "IOBUF PORT %q %s;\n", port, lpfAttrs(p.Attrs))
	}
	return b.String(), nil
}

// lpfAttrs renders the IOBUF attribute string for a pin. IO_TYPE comes from
// the iostandard attr (defaulting to LVCMOS33); then DRIVE, SLEWRATE, PULLMODE
// are appended in that stable order when present in attrs.
func lpfAttrs(attrs map[string]design.Value) string {
	iostd := "LVCMOS33"
	if v, ok := attrs["iostandard"]; ok && v.Text != "" {
		iostd = strings.ToUpper(v.Text)
	}
	var sb strings.Builder
	sb.WriteString("IO_TYPE=")
	sb.WriteString(iostd)
	// stable order: DRIVE, SLEWRATE, PULLMODE
	if v, ok := attrs["drive"]; ok {
		if v.Kind == design.KindInt {
			fmt.Fprintf(&sb, " DRIVE=%d", v.Int)
		} else if v.Text != "" {
			fmt.Fprintf(&sb, " DRIVE=%s", v.Text)
		}
	}
	if v, ok := attrs["slewrate"]; ok && v.Text != "" {
		fmt.Fprintf(&sb, " SLEWRATE=%s", strings.ToUpper(v.Text))
	}
	if v, ok := attrs["pullmode"]; ok && v.Text != "" {
		fmt.Fprintf(&sb, " PULLMODE=%s", strings.ToUpper(v.Text))
	}
	return sb.String()
}
