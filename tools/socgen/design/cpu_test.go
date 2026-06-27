package design

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadCPUBlock(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "design.yaml")
	os.WriteFile(p, []byte(`target: ecp5
cpu:
  architecture: one_cpu_m0
  cores: 1
  model: j4
  decode: rom
  copro: false
  cache: id
`), 0o644)
	d, err := Load(p)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if d.CPU == nil {
		t.Fatal("CPU block not parsed")
	}
	if d.CPU.Model != "j4" || d.CPU.Decode != "rom" || d.CPU.Cache != "id" ||
		d.CPU.Architecture != "one_cpu_m0" || d.CPU.Cores != 1 || d.CPU.Copro {
		t.Errorf("unexpected CPU block: %+v", d.CPU)
	}
}
