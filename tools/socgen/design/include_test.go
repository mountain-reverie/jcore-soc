package design

import (
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
	d, errs := Load(filepath.Join(dir, "design.yaml"))
	if len(errs) != 0 {
		t.Fatalf("errs: %v", errs)
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
	d, errs := Load(filepath.Join(dir, "design.yaml"))
	if len(errs) != 0 {
		t.Fatalf("errs: %v", errs)
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
	d, errs := Load(filepath.Join(dir, "design.yaml"))
	if len(errs) != 0 {
		t.Fatalf("errs: %v", errs)
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
	_, errs := Load(filepath.Join(dir, "a.yaml"))
	if len(errs) == 0 {
		t.Fatal("expected an include-cycle error")
	}
}
