package generate

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteCreatesFiles(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "out") // a not-yet-existing subdir
	files := []File{
		{Name: "devices.vhd", Content: "entity devices"},
		{Name: "build.mk", Content: "$(VHDLS) += devices.vhd\n"},
	}
	if err := Write(files, dir); err != nil {
		t.Fatalf("Write: %v", err)
	}
	for _, f := range files {
		got, err := os.ReadFile(filepath.Join(dir, f.Name))
		if err != nil {
			t.Fatalf("read %s: %v", f.Name, err)
		}
		if string(got) != f.Content {
			t.Errorf("%s content = %q, want %q", f.Name, got, f.Content)
		}
	}
}

func TestWriteRejectsNonDir(t *testing.T) {
	// A path that exists as a file, not a directory.
	f := filepath.Join(t.TempDir(), "afile")
	if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Write([]File{{Name: "x", Content: "y"}}, f); err == nil {
		t.Errorf("expected error writing into a non-directory path")
	}
}
