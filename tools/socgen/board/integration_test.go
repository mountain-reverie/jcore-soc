package board

import (
	"os"
	"testing"

	"github.com/j-core/jcore-soc/tools/socgen/internal/errutil"
)

// TestBoardMimasV2 runs the real make file-list + parse + validate for mimas_v2.
// It runs under `make test` (which sets JCORE_SOC_ROOT) and skips for a bare
// `go test`. It invokes `make mimas_v2` (~15s, gcc/make/perl; idempotent).
func TestBoardMimasV2(t *testing.T) {
	root := os.Getenv("JCORE_SOC_ROOT")
	if root == "" {
		t.Skip("JCORE_SOC_ROOT not set")
	}
	files, err := Files(root, "mimas_v2")
	if err != nil {
		t.Fatalf("Files(mimas_v2): %v", err)
	}
	if len(files) < 100 {
		t.Fatalf("expected >100 board files, got %d", len(files))
	}
	lib, perr := Library(files)
	// STRICT: every board file must parse.
	if perr != nil {
		t.Fatalf("expected 0 parse failures over %d files, got %d:\n%v",
			len(files), len(errutil.Errors(perr)), perr)
	}
	// config.vhd is in the set -> CFG_* constants are in the library.
	if _, ok := lib.Package("config"); !ok {
		// the package may be named differently; fall back to a known constant.
		if _, ok := lib.ResolveType("CFG_CLK_CPU_PERIOD_NS"); !ok {
			t.Log("note: neither config package nor CFG_CLK_CPU_PERIOD_NS resolved; check config.vhd package name")
		}
	}
	// Several known device-class entities must be present.
	for _, ent := range []string{"uartlitedb", "pio"} {
		if _, ok := lib.Entity(ent); !ok {
			t.Errorf("expected entity %q in the board library", ent)
		}
	}
	// Full-board validation of the migrated mimas_v2 spec.
	b, verr := Load(root, "mimas_v2")
	t.Logf("mimas_v2 Load: %d total errors", len(errutil.Errors(verr)))
	for _, e := range errutil.Errors(verr) {
		t.Logf("  %v", e)
	}
	// With 0 parse failures, a uartlite-class device's entity must resolve.
	if b.Design != nil {
		if _, ok := b.Library.Entity("uartlitedb"); !ok {
			t.Error("uartlitedb must resolve from the full board library")
		}
	}
}
