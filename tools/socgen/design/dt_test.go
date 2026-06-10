package design

import (
	"testing"

	"gopkg.in/yaml.v3"
)

func TestDtPropsOrder(t *testing.T) {
	var p DtProps
	src := "compatible: jcore,spi2\n\"#address-cells\": [1]\nspi-max-frequency: [25000000]\nmode: [0]\n"
	if err := yaml.Unmarshal([]byte(src), &p); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	wantKeys := []string{"compatible", "#address-cells", "spi-max-frequency", "mode"}
	if len(p) != len(wantKeys) {
		t.Fatalf("len = %d, want %d (%v)", len(p), len(wantKeys), p)
	}
	for i, k := range wantKeys {
		if p[i].Key != k {
			t.Errorf("p[%d].Key = %q, want %q (source order, not sorted)", i, p[i].Key, k)
		}
	}
	if v, ok := p.Get("compatible"); !ok || v != "jcore,spi2" {
		t.Errorf("Get(compatible) = %v,%v; want jcore,spi2,true", v, ok)
	}
	if _, ok := p.Get("nope"); ok {
		t.Errorf("Get(nope) should be false")
	}
}

func TestDtChildDecode(t *testing.T) {
	src := `- sdcard@0
- properties:
    compatible: mmc-spi-slot
    reg: [0]
  children:
    - - partition@0
      - properties:
          label: spi_flash
`
	var c DtChild
	if err := yaml.Unmarshal([]byte(src), &c); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if c.Name != "sdcard@0" {
		t.Errorf("Name = %q, want sdcard@0", c.Name)
	}
	if len(c.Props) != 2 || c.Props[0].Key != "compatible" || c.Props[1].Key != "reg" {
		t.Errorf("Props = %v, want [compatible reg] in order", c.Props)
	}
	if len(c.Children) != 1 || c.Children[0].Name != "partition@0" {
		t.Fatalf("Children = %v, want [partition@0]", c.Children)
	}
	if len(c.Children[0].Props) != 1 || c.Children[0].Props[0].Key != "label" {
		t.Errorf("nested Props = %v, want [label]", c.Children[0].Props)
	}
}
