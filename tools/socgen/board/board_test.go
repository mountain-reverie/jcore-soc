package board

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/j-core/jcore-soc/tools/socgen/internal/errutil"
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
	lib, err := Library([]string{good1, good2, bad})
	if len(errutil.Errors(err)) != 1 {
		t.Fatalf("want 1 parse error (the broken file), got %d: %v", len(errutil.Errors(err)), err)
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
	missing := filepath.Join(dir, "missing.txt")
	_, err = readFileList(missing)
	if err == nil {
		t.Fatal("missing list should error")
	}
	if !errors.Is(err, ErrReadList) {
		t.Errorf("missing list: want ErrReadList, got %v", err)
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("missing list: want os.ErrNotExist reachable via Unwrap, got %v", err)
	}
	var fle *FileListError
	if !errors.As(err, &fle) {
		t.Fatalf("missing list: want *FileListError, got %T", err)
	}
	if fle.Target != missing {
		t.Errorf("Target = %q want %q", fle.Target, missing)
	}
}

func TestReadFileListEmpty(t *testing.T) {
	dir := t.TempDir()
	p := writeFile(t, dir, "vhdl_list.txt", "notes.txt\n\nREADME\n")
	_, err := readFileList(p)
	if err == nil {
		t.Fatal("a list with no .vhd/.vhh entries should return an error")
	}
	if !errors.Is(err, ErrEmptyList) {
		t.Errorf("empty list: want ErrEmptyList, got %v", err)
	}
	// message smoke for *FileListError (no underlying Err on ErrEmptyList).
	if got := err.Error(); got != "empty file list ("+p+")" {
		t.Errorf("Error() = %q", got)
	}
}

func TestLibraryEmpty(t *testing.T) {
	lib, err := Library(nil)
	if lib == nil {
		t.Fatal("Library(nil) should return a non-nil *iface.Library")
	}
	if err != nil {
		t.Errorf("Library(nil) should have zero errors, got: %v", err)
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
	b, err := loadFrom(root, "tb", []string{ent})
	if err != nil {
		t.Fatalf("expected clean load+validate, got: %v", err)
	}
	if b.Design == nil || len(b.Design.Devices) != 1 {
		t.Fatalf("design = %+v", b.Design)
	}
	if _, ok := b.Library.Entity("uartlitedb"); !ok {
		t.Error("library should contain uartlitedb")
	}
}
