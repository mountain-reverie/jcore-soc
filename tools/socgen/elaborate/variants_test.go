package elaborate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestLoadVariantsRejectsMMUArch mirrors the submodule parser's guard
// (components/cpu/decode/gen-go/internal/variants/variants.go: "sets
// MMU_ARCH; PRIV_ARCH implies MMU") on socgen's own loadVariants(). Without
// this, a variants.toml adding MMU_ARCH to a variant fails loudly in cpugen
// but would propagate silently into the SoC's generic map, since socgen's
// parser was a bare toml.DecodeFile with no validation of its own.
func TestLoadVariantsRejectsMMUArch(t *testing.T) {
	dir := t.TempDir()
	cpuDir := filepath.Join(dir, "components", "cpu")
	if err := os.MkdirAll(cpuDir, 0o755); err != nil {
		t.Fatal(err)
	}
	toml := `
[j4]
generics    = { PRIV_ARCH = "true", MMU_ARCH = "true" }
overlay     = "sh4"
extra_files = ["core/tlb.vhd"]
config_file = "core/cpu_config_j4.vhd"
`
	if err := os.WriteFile(filepath.Join(cpuDir, "variants.toml"), []byte(toml), 0o644); err != nil {
		t.Fatal(err)
	}

	SetRoot(dir)
	defer SetRoot("")

	_, err := loadVariants()
	if err == nil {
		t.Fatal("expected error for a variant setting MMU_ARCH, got nil")
	}
	if !strings.Contains(err.Error(), "MMU_ARCH") {
		t.Errorf("error %q does not mention MMU_ARCH", err)
	}
}

// TestLoadVariantsRequiresConfigFile mirrors the submodule parser's other
// guard: every variant must set config_file.
func TestLoadVariantsRequiresConfigFile(t *testing.T) {
	dir := t.TempDir()
	cpuDir := filepath.Join(dir, "components", "cpu")
	if err := os.MkdirAll(cpuDir, 0o755); err != nil {
		t.Fatal(err)
	}
	toml := `
[j9]
generics    = {}
overlay     = ""
extra_files = []
`
	if err := os.WriteFile(filepath.Join(cpuDir, "variants.toml"), []byte(toml), 0o644); err != nil {
		t.Fatal(err)
	}

	SetRoot(dir)
	defer SetRoot("")

	_, err := loadVariants()
	if err == nil {
		t.Fatal("expected error for a variant missing config_file, got nil")
	}
	if !strings.Contains(err.Error(), "config_file") {
		t.Errorf("error %q does not mention config_file", err)
	}
}

// TestLoadVariantsAcceptsWellFormed is a sanity check that a valid
// variants.toml still loads cleanly through the same validation path.
func TestLoadVariantsAcceptsWellFormed(t *testing.T) {
	dir := t.TempDir()
	cpuDir := filepath.Join(dir, "components", "cpu")
	if err := os.MkdirAll(cpuDir, 0o755); err != nil {
		t.Fatal(err)
	}
	toml := `
[j4]
generics    = { PRIV_ARCH = "true" }
overlay     = "sh4"
extra_files = ["core/tlb.vhd"]
config_file = "core/cpu_config_j4.vhd"
`
	if err := os.WriteFile(filepath.Join(cpuDir, "variants.toml"), []byte(toml), 0o644); err != nil {
		t.Fatal(err)
	}

	SetRoot(dir)
	defer SetRoot("")

	v, err := loadVariants()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v["j4"].Generics["PRIV_ARCH"] != "true" {
		t.Errorf("j4 generics = %v, want PRIV_ARCH=true", v["j4"].Generics)
	}
}
