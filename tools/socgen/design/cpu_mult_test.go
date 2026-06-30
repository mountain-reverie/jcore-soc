package design

import (
	"testing"

	"gopkg.in/yaml.v3"
)

func TestCPUMultField(t *testing.T) {
	var c CPU
	if err := yaml.Unmarshal([]byte("model: j1\ndecode: rom\nmult: dsp\n"), &c); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if c.Mult != "dsp" {
		t.Fatalf("Mult = %q, want %q", c.Mult, "dsp")
	}
	var d CPU
	if err := yaml.Unmarshal([]byte("model: j1\ndecode: rom\n"), &d); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if d.Mult != "" {
		t.Fatalf("absent mult: Mult = %q, want \"\"", d.Mult)
	}
}
