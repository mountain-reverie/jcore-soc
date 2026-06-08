package emit

import (
	"os"
	"strings"
	"testing"

	"github.com/j-core/jcore-soc/tools/socgen/board"
	"github.com/j-core/jcore-soc/tools/socgen/elaborate"
	"github.com/j-core/jcore-soc/tools/socgen/vhdl"
)

// TestEmitDevicesMimasV2 runs the full pipeline (board.Load -> Elaborate ->
// emit.Devices) on the real mimas_v2 board and proves the emitted devices.vhd
// round-trips through our own parser with every bound device instantiated and
// top/padring entities NOT instantiated (they belong in soc.vhd / pad_ring.vhd
// since P5c-ii). It runs under `make test` (which sets JCORE_SOC_ROOT) and skips
// for a bare `go test`.
func TestEmitDevicesMimasV2(t *testing.T) {
	root := os.Getenv("JCORE_SOC_ROOT")
	if root == "" {
		t.Skip("JCORE_SOC_ROOT not set")
	}
	b, err := board.Load(root, "mimas_v2")
	if err != nil {
		t.Fatalf("board.Load: %v", err)
	}
	if b.Design == nil || b.Library == nil {
		t.Fatalf("board.Load returned incomplete board: %v", err)
	}
	res, eerr := elaborate.Elaborate(b)
	if eerr != nil {
		// elaborate is best-effort here; the round-trip + instance checks below
		// validate emit. Log resolution notes but don't fail on them.
		t.Logf("elaborate notes: %v", eerr)
	}
	if res == nil {
		t.Fatalf("elaborate.Elaborate returned nil resolution")
	}

	out, derr := Devices(res)
	if derr != nil {
		// emit is best-effort; log emit notes (e.g. unbound entities) but don't fail here.
		t.Logf("emit notes: %v", derr)
	}
	t.Logf("mimas_v2 devices.vhd: %d bytes, %d lines", len(out), strings.Count(out, "\n"))

	// round-trip: must re-parse with zero errors.
	df, perr := vhdl.ParseFile(vhdl.NewFileSet(), "devices.vhd", []byte(out))
	if perr != nil {
		t.Fatalf("emitted devices.vhd does not re-parse: %v\n---\n%s", perr, out)
	}

	// collect instantiation labels from the re-parsed AST.
	labels := map[string]bool{}
	for _, u := range df.Units {
		if a, ok := u.(*vhdl.ArchitectureBody); ok {
			for _, s := range a.Stmts {
				if i, ok := s.(*vhdl.InstantiationStmt); ok {
					labels[i.Label] = true
				}
			}
		}
	}

	// every BOUND device is instantiated; unbound ones are legitimately skipped.
	var bound, skipped, tops, pads int
	for _, dev := range res.Devices {
		rc := res.Classes[lc(dev.Class)]
		if rc == nil || rc.Entity == nil {
			t.Logf("device %q has unbound class %q — skipped (expected)", dev.Name, dev.Class)
			skipped++
			continue
		}
		bound++
		if !labels[lc(dev.Name)] {
			t.Errorf("bound device %q not instantiated", dev.Name)
		}
	}
	for name := range res.TopEntities {
		tops++
		if labels[lc(name)] {
			t.Errorf("top-entity %q must NOT be instantiated in devices.vhd (P5c-ii moved it to soc.vhd)", name)
		}
	}
	for name := range res.PadringEntities {
		pads++
		if labels[lc(name)] {
			t.Errorf("padring-entity %q must NOT be instantiated in devices.vhd", name)
		}
	}
	t.Logf("mimas_v2 instances: %d bound devices (%d skipped unbound), %d top, %d padring; %d labels in AST",
		bound, skipped, tops, pads, len(labels))

	// Guard against a vacuous pass: a board-spec or pipeline regression that
	// silently resolves nothing would make every loop above a no-op.
	if bound+tops+pads == 0 {
		t.Fatalf("no devices/top/padring entities resolved for mimas_v2; pipeline regression?")
	}
}
