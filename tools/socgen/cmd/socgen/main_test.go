package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRunRequiresBoardArg(t *testing.T) {
	if err := run([]string{}); err == nil {
		t.Errorf("expected error with no board argument")
	}
}

func TestRunHelpIsNotAnError(t *testing.T) {
	// flag prints usage on -h; run should treat it as a clean exit, not an error.
	if err := run([]string{"-h"}); err != nil {
		t.Errorf("run -h = %v, want nil", err)
	}
}

func TestRunMimasV2(t *testing.T) {
	root := os.Getenv("JCORE_SOC_ROOT")
	if root == "" {
		t.Skip("JCORE_SOC_ROOT not set")
	}
	out := t.TempDir()
	if err := run([]string{"-root", root, "-o", out, "mimas_v2"}); err != nil {
		t.Fatalf("run: %v", err)
	}
	for _, name := range []string{"devices.vhd", "soc.vhd", "pad_ring.vhd", "board.dts", "board.h", "build.mk"} {
		if _, err := os.Stat(filepath.Join(out, name)); err != nil {
			t.Errorf("expected %s written: %v", name, err)
		}
	}
}
