package design

import (
	"testing"

	"gopkg.in/yaml.v3"
)

func TestPioParse(t *testing.T) {
	src := `
data-bus-decode: simple
pio:
  "[0 7]": { name: led }
  "[8 15]": { name: sevensegment }
  "16": { name: sevensegmentenable }
  "[19 31]": 0
`
	var s System
	if err := yaml.Unmarshal([]byte(src), &s); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(s.Pio) != 4 {
		t.Fatalf("want 4 pio entries, got %d: %+v", len(s.Pio), s.Pio)
	}
	byLo := map[int]PioEntry{}
	for _, e := range s.Pio {
		byLo[e.Lo] = e
	}
	if e := byLo[0]; e.Hi != 7 || e.Const != nil || e.Name != "led" {
		t.Errorf("[0 7] = %+v, want Lo0 Hi7 name=led const=nil", e)
	}
	if e := byLo[16]; e.Lo != 16 || e.Hi != 16 || e.Name != "sevensegmentenable" || e.Const != nil {
		t.Errorf("single key 16 = %+v, want Lo16 Hi16 name=sevensegmentenable const=nil", e)
	}
	if e := byLo[19]; e.Hi != 31 || e.Const == nil || *e.Const != 0 {
		t.Errorf("[19 31] = %+v, want Hi31 const=0", e)
	}
}

func TestPioParseErrors(t *testing.T) {
	for _, bad := range []string{
		"pio:\n  \"[0 7\": { name: x }\n",   // unclosed bracket
		"pio:\n  \"[0 7 8]\": { name: x }\n", // too many range fields
		"pio:\n  \"[a b]\": { name: x }\n",   // non-int range
		"pio:\n  \"foo\": 0\n",               // non-int single key
		"pio:\n  \"5\": [1, 2]\n",            // value neither int nor map
	} {
		var s System
		if err := yaml.Unmarshal([]byte(bad), &s); err == nil {
			t.Errorf("expected parse error for %q, got nil", bad)
		}
	}
}
