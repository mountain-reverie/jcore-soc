package board

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/j-core/jcore-soc/tools/socgen/design"
	"github.com/j-core/jcore-soc/tools/socgen/iface"
)

// Board is a loaded + validated board: its parsed spec and the interface
// Library extracted from its full VHDL file set.
type Board struct {
	Name    string
	Design  *design.Design
	Library *iface.Library
}

var vhdlExt = regexp.MustCompile(`\.vh[hd]$`)

// readFileList reads a vhdl_list.txt (one path per line) and returns the lines
// naming a .vhd/.vhh file. Separated from Files so it is unit-testable.
func readFileList(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read file list %s: %w", path, err)
	}
	var out []string
	for _, ln := range strings.Split(string(data), "\n") {
		ln = strings.TrimSpace(ln)
		if ln != "" && vhdlExt.MatchString(ln) {
			out = append(out, ln)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("empty file list %s", path)
	}
	return out, nil
}

// loadFrom composes spec-load + library-build + validate for a board whose VHDL
// file set is already known (no make). Load (Task 2) wraps it with Files.
func loadFrom(root, name string, files []string) (*Board, []error) {
	d, derr := design.Load(filepath.Join(root, "targets", "boards", name, "design.yaml"))
	lib, lerrs := Library(files)
	errs := append([]error{}, lerrs...)
	if derr != nil {
		errs = append(errs, derr)
	}
	if d != nil {
		if verr := design.Validate(d, lib); verr != nil {
			errs = append(errs, verr)
		}
	}
	return &Board{Name: name, Design: d, Library: lib}, errs
}

// Files runs the build's file-list target for the named board (root = the
// jcore-soc repo root) and returns the absolute VHDL file paths it lists.
func Files(root, name string) ([]string, error) {
	cmd := exec.Command("make", "-C", root, name,
		"TARGET=vhdl_list.txt", "LAST_OUTPUT=false", "REL_OUTPUT_DIR=output/"+name)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("make %s vhdl_list.txt: %w: %s", name, err, tailStr(stderr.String()))
	}
	return readFileList(filepath.Join(root, "output", name, "vhdl_list.txt"))
}

// Load loads the board's YAML spec, builds the interface Library from its full
// VHDL file set (via make), and validates the spec against it. Returns a
// best-effort Board plus all collected errors (load + parse + validation).
func Load(root, name string) (*Board, []error) {
	files, err := Files(root, name)
	if err != nil {
		d, derr := design.Load(filepath.Join(root, "targets", "boards", name, "design.yaml"))
		var errs []error
		if derr != nil {
			errs = append(errs, derr)
		}
		return &Board{Name: name, Design: d}, append(errs, err)
	}
	return loadFrom(root, name, files)
}

func tailStr(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 600 {
		return "..." + s[len(s)-600:]
	}
	return s
}
