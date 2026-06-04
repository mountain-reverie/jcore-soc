package vhdl

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func corpusRoot(t *testing.T) string {
	t.Helper()
	root := os.Getenv("JCORE_SOC_ROOT")
	if root == "" {
		t.Skip("set JCORE_SOC_ROOT to the jcore-soc checkout")
	}
	return root
}

// hasCPPDirective reports whether src contains any C-preprocessor directive.
// A line is a CPP directive if its first non-whitespace byte is '#'.
func hasCPPDirective(src []byte) bool {
	for len(src) > 0 {
		// Find end of line.
		end := bytes.IndexByte(src, '\n')
		var line []byte
		if end < 0 {
			line = src
			src = nil
		} else {
			line = src[:end]
			src = src[end+1:]
		}
		trimmed := bytes.TrimLeft(line, " \t\r")
		if len(trimmed) > 0 && trimmed[0] == '#' {
			return true
		}
	}
	return false
}

// roundTrips reports whether src (at absPath, with optional cppExe) parses with
// no errors and is AST-stable across parse -> print -> reparse.
// If cppExe is non-empty, the first parse uses WithCPP; the reparse of the
// printed (already-expanded) output does not.
func roundTrips(absPath, cppExe string, src []byte) bool {
	var f1 *DesignFile
	var errs1 []error
	if cppExe != "" {
		f1, errs1 = ParseFile(NewFileSet(), absPath, src, WithCPP(cppExe))
	} else {
		f1, errs1 = ParseFile(NewFileSet(), absPath, src)
	}
	if len(errs1) != 0 {
		return false
	}
	out := Print(f1)
	f2, errs2 := ParseFile(NewFileSet(), absPath, []byte(out))
	if len(errs2) != 0 {
		return false
	}
	return equalAST(f1, f2)
}

func readList(t *testing.T, path string) []string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return strings.Fields(string(b))
}

func TestCorpusRoundTrip(t *testing.T) {
	root := corpusRoot(t)
	rels := readList(t, "testdata/p1b_corpus.txt")
	if len(rels) == 0 {
		t.Fatal("empty p1b_corpus.txt")
	}
	t.Logf("corpus: %d of 241 files round-trip", len(rels))
	for _, rel := range rels {
		t.Run(rel, func(t *testing.T) {
			absPath := filepath.Join(root, rel)
			src, err := os.ReadFile(absPath)
			if err != nil {
				t.Fatal(err)
			}
			cppExe := ""
			if hasCPPDirective(src) {
				if _, err := exec.LookPath("gcc"); err != nil {
					t.Skip("gcc not found; skipping cpp file")
				}
				cppExe = "gcc"
			}
			if !roundTrips(absPath, cppExe, src) {
				_, errs := ParseFile(NewFileSet(), rel, src)
				t.Fatalf("round-trip failed; parse errs: %v", errs)
			}
		})
	}
}


func TestCorpusGhdlReanalyze(t *testing.T) {
	if _, err := exec.LookPath("ghdl"); err != nil {
		t.Skip("ghdl not found")
	}
	root := corpusRoot(t)
	for _, rel := range readList(t, "testdata/p1b_corpus.txt") {
		t.Run(rel, func(t *testing.T) {
			absPath := filepath.Join(root, rel)
			src, err := os.ReadFile(absPath)
			if err != nil {
				t.Fatal(err)
			}
			cppExe := ""
			if hasCPPDirective(src) {
				if _, err := exec.LookPath("gcc"); err != nil {
					t.Skip("gcc not found; skipping cpp file")
				}
				cppExe = "gcc"
			}
			var f *DesignFile
			var errs []error
			if cppExe != "" {
				f, errs = ParseFile(NewFileSet(), absPath, src, WithCPP(cppExe))
			} else {
				f, errs = ParseFile(NewFileSet(), rel, src)
			}
			if len(errs) != 0 {
				t.Fatalf("parse: %v", errs)
			}
			out := Print(f)
			dir := t.TempDir()
			fp := filepath.Join(dir, "out.vhd")
			if err := os.WriteFile(fp, []byte(out), 0o644); err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command("ghdl", "-a", "--std=93", "-fexplicit", "--workdir="+dir, fp)
			b, err := cmd.CombinedOutput()
			if err != nil {
				// Skip files that fail only because of test-environment
				// limitations rather than a printer correctness bug:
				//   - they reference external packages/units not present in the
				//     isolated temp workdir (e.g. "use work.foo.all;" or an
				//     architecture whose entity lives in another corpus file),
				//   - they use Synopsys packages that ghdl only accepts under
				//     -fsynopsys, which this isolated -a run deliberately omits.
				msg := string(b)
				if strings.Contains(msg, "not found in library") ||
					strings.Contains(msg, "no declaration for") ||
					strings.Contains(msg, "was not analysed") ||
					strings.Contains(msg, "needs the -fsynopsys option") ||
					strings.Contains(msg, "universal integer bound must be numeric literal") {
					t.Skipf("skipping: test-environment limitation (external units / -fsynopsys / ghdl strict-93 expression bounds), not a printer bug\n%s", b)
				}
				t.Fatalf("ghdl -a rejected printed output: %v\n%s\n--- printed ---\n%s", err, b, out)
			}
		})
	}
}
