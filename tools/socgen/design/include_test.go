package design

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// writeFiles writes a map of relative-path->content under a temp dir and returns it.
func writeFiles(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for rel, content := range files {
		p := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestIncludeWholeFile(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		"design.yaml":  "device-classes: !include classes.yaml\n",
		"classes.yaml": "uartlite:\n  entity: uartlitedb\n  left-addr-bit: 3\n",
	})
	d, err := Load(filepath.Join(dir, "design.yaml"))
	if err != nil {
		t.Fatalf("errs: %v", err)
	}
	c, ok := d.DeviceClasses["uartlite"]
	if !ok || c.Entity != "uartlitedb" || c.LeftAddrBit != 3 {
		t.Fatalf("class = %+v ok=%v", c, ok)
	}
}

func TestIncludeWithOverride(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		"design.yaml": `devices:
  - <<: !include aic.yaml
    generics: { rtc_sec_length34b: true }
`,
		"aic.yaml": `class: aic
name: aic0
generics: { rtc_sec_length34b: false, c_busperiod: CFG_CLK_CPU_PERIOD_NS }
`,
	})
	d, err := Load(filepath.Join(dir, "design.yaml"))
	if err != nil {
		t.Fatalf("errs: %v", err)
	}
	dev := d.Devices[0]
	if dev.Class != "aic" || dev.Name != "aic0" {
		t.Fatalf("device = %+v", dev)
	}
	// deep merge: the override wins, the other generic is preserved
	if dev.Generics["rtc_sec_length34b"].Bool != true {
		t.Errorf("override not applied: %+v", dev.Generics["rtc_sec_length34b"])
	}
	if dev.Generics["c_busperiod"].Text != "CFG_CLK_CPU_PERIOD_NS" {
		t.Errorf("inherited generic lost: %+v", dev.Generics)
	}
}

func TestRemoveKeyOnMerge(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		"design.yaml": `devices:
  - <<: !include base.yaml
    generics: { drop_me: !remove }
`,
		"base.yaml": "class: x\ngenerics: { drop_me: 1, keep_me: 2 }\n",
	})
	d, err := Load(filepath.Join(dir, "design.yaml"))
	if err != nil {
		t.Fatalf("errs: %v", err)
	}
	g := d.Devices[0].Generics
	if _, present := g["drop_me"]; present {
		t.Errorf("drop_me should be removed: %+v", g)
	}
	if g["keep_me"].Int != 2 {
		t.Errorf("keep_me = %+v", g["keep_me"])
	}
}

func TestIncludeCycle(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		"a.yaml": "x: !include b.yaml\n",
		"b.yaml": "y: !include a.yaml\n",
	})
	_, err := Load(filepath.Join(dir, "a.yaml"))
	if !errors.Is(err, ErrIncludeCycle) {
		t.Fatalf("expected an include-cycle error, got %v", err)
	}
}

func TestSeqConcatOnMerge(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		"design.yaml": "merge-signals:\n  <<: !include base.yaml\n  sig: [c]\n",
		"base.yaml":   "sig: [a, b]\n",
	})
	d, err := Load(filepath.Join(dir, "design.yaml"))
	if err != nil {
		t.Fatalf("errs: %v", err)
	}
	// sequences concatenate, override (sibling) items first then base.
	got := d.MergeSignals["sig"]
	want := []string{"c", "a", "b"}
	if len(got) != 3 || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
		t.Errorf("sig = %v, want %v", got, want)
	}
}

func TestDepthCap(t *testing.T) {
	files := map[string]string{}
	for i := 0; i < 20; i++ {
		files[fmt.Sprintf("f%d.yaml", i)] = fmt.Sprintf("x: !include f%d.yaml\n", i+1)
	}
	files["f20.yaml"] = "x: leaf\n"
	dir := writeFiles(t, files)
	_, err := Load(filepath.Join(dir, "f0.yaml"))
	if !errors.Is(err, ErrIncludeDepth) {
		t.Fatalf("expected an include-depth error, got %v", err)
	}
}

func TestNonMappingMergeError(t *testing.T) {
	_, err := loadString(t, "devices:\n  - class: c\n    <<: 5\n")
	var se *SpecError
	if !errors.As(err, &se) || se.Msg != "<< value must be a mapping" {
		t.Fatalf("expected a non-mapping << SpecError, got %v", err)
	}
}

func TestStandaloneRemove(t *testing.T) {
	d, err := loadString(t, "devices:\n  - class: c\n    name: !remove\n")
	if err != nil {
		t.Fatalf("errs: %v", err)
	}
	if d.Devices[0].Name != "" {
		t.Errorf("standalone !remove should drop the key; name = %q", d.Devices[0].Name)
	}
}
