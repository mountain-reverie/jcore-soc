package board

import (
	"os"
	"path/filepath"
	"testing"
)

func writeFile(t *testing.T, dir, rel, content string) string {
	t.Helper()
	p := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLibraryBestEffort(t *testing.T) {
	dir := t.TempDir()
	good1 := writeFile(t, dir, "a.vhd", "entity ea is port (clk : in std_logic); end entity;")
	good2 := writeFile(t, dir, "b.vhd", "package pb is constant K : integer := 1; end package;")
	bad := writeFile(t, dir, "c.vhd", "entity broken is port (")
	lib, errs := Library([]string{good1, good2, bad})
	if len(errs) != 1 {
		t.Fatalf("want 1 parse error (the broken file), got %d: %v", len(errs), errs)
	}
	if _, ok := lib.Entity("ea"); !ok {
		t.Error("entity ea should be extracted despite the broken file")
	}
	if _, ok := lib.Package("pb"); !ok {
		t.Error("package pb should be extracted")
	}
}

func TestReadFileList(t *testing.T) {
	dir := t.TempDir()
	p := writeFile(t, dir, "vhdl_list.txt",
		"/abs/x.vhd\n/abs/y.vhh\n/abs/notes.txt\n\n  /abs/z.vhd  \n")
	got, err := readFileList(p)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"/abs/x.vhd", "/abs/y.vhh", "/abs/z.vhd"}
	if len(got) != len(want) {
		t.Fatalf("got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("line %d: got %q want %q", i, got[i], want[i])
		}
	}
	if _, err := readFileList(filepath.Join(dir, "missing.txt")); err == nil {
		t.Error("missing list should error")
	}
}

func TestReadFileListEmpty(t *testing.T) {
	dir := t.TempDir()
	p := writeFile(t, dir, "vhdl_list.txt", "notes.txt\n\nREADME\n")
	if _, err := readFileList(p); err == nil {
		t.Error("a list with no .vhd/.vhh entries should return an error")
	}
}

func TestLibraryEmpty(t *testing.T) {
	lib, errs := Library(nil)
	if lib == nil {
		t.Fatal("Library(nil) should return a non-nil *iface.Library")
	}
	if len(errs) != 0 {
		t.Errorf("Library(nil) should have zero errors, got: %v", errs)
	}
}

func TestLoadFromComposition(t *testing.T) {
	root := t.TempDir()
	// a tiny board spec referencing one device class -> one entity
	writeFile(t, root, "targets/boards/tb/design.yaml", `device-classes:
  uartlite: { entity: uartlitedb }
devices:
  - { class: uartlite, name: uart0 }
`)
	ent := writeFile(t, root, "vhdl/uartlitedb.vhd",
		"entity uartlitedb is port (clk : in std_logic); end entity;")
	b, errs := loadFrom(root, "tb", []string{ent})
	if len(errs) != 0 {
		t.Fatalf("expected clean load+validate, got: %v", errs)
	}
	if b.Design == nil || len(b.Design.Devices) != 1 {
		t.Fatalf("design = %+v", b.Design)
	}
	if _, ok := b.Library.Entity("uartlitedb"); !ok {
		t.Error("library should contain uartlitedb")
	}
}
