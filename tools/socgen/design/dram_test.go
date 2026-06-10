package design

import (
	"testing"

	"gopkg.in/yaml.v3"
)

func TestDramParse(t *testing.T) {
	var s System
	if err := yaml.Unmarshal([]byte("dram: [0x10000000, 0x4000000]\n"), &s); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if s.Dram[0] != 0x10000000 || s.Dram[1] != 0x4000000 {
		t.Errorf("dram = %#x, want [0x10000000 0x4000000]", s.Dram)
	}
}
