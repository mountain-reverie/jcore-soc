package elaborate

import (
	"testing"

	"github.com/j-core/jcore-soc/tools/socgen/design"
)

// resolveResult bundles the two outputs of resolvePins for entity-pad tests.
type resolveResult struct {
	Pins            []*ResolvedPin
	EntityBoundPads map[string]bool
}

// findPin returns the ResolvedPin with Net==name, or nil.
func findPin(pins []*ResolvedPin, name string) *ResolvedPin {
	for _, p := range pins {
		if p.Net == name {
			return p
		}
	}
	return nil
}

func TestEntityPadResolvesToBufEntity(t *testing.T) {
	tru := true
	d := &design.Design{
		Pins: &design.PinsSpec{
			Rules: []*design.PinRule{
				{
					Match:     &design.Match{Regex: "sdram_d"},
					Signal:    &design.SigSpec{Kind: design.SigName, Name: "sdram_d"},
					EntityPad: &tru,
				},
			},
			Pins: []*design.Pin{{Net: "sdram_d", Pad: "J16"}},
		},
	}
	pins, boundPads := resolvePins(d, map[string]*Signal{})
	res := resolveResult{Pins: pins, EntityBoundPads: boundPads}

	rp := findPin(res.Pins, "sdram_d")
	if rp == nil {
		t.Fatal("no resolved pin sdram_d")
	}
	if rp.BufferKind != BufEntity {
		t.Errorf("BufferKind = %v, want BufEntity", rp.BufferKind)
	}
	if rp.PadDir != "inout" {
		t.Errorf("PadDir = %q, want inout", rp.PadDir)
	}
	if rp.Signal != "sdram_d" {
		t.Errorf("Signal = %q, want sdram_d", rp.Signal)
	}
	if res.EntityBoundPads == nil || !res.EntityBoundPads["sdram_d"] {
		t.Errorf("EntityBoundPads should contain sdram_d; got %v", res.EntityBoundPads)
	}
}
