package design

import (
	"os"
	"path/filepath"
	"testing"
)

func TestVariantIncludeMerges(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "base.yaml"), []byte("target: ecp5\nsystem:\n  dram: [0x10000000, 0x2000000]\n"), 0o644)
	// Standard YAML merge-key form: a bare root-level `!include` followed by
	// sibling keys is not valid YAML (the tagged scalar terminates the node),
	// so variant files merge the base via `<<: !include base.yaml`.
	os.WriteFile(filepath.Join(dir, "design.v.yaml"), []byte("<<: !include base.yaml\ncpu:\n  architecture: one_cpu_m0\n  cores: 1\n  model: j2\n  decode: direct\n  cache: id\n"), 0o644)
	d, err := Load(filepath.Join(dir, "design.v.yaml"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if d.Target != "ecp5" || d.CPU == nil || d.System == nil {
		t.Fatalf("merge lost keys: target=%q cpu=%v system=%v", d.Target, d.CPU, d.System)
	}
}
