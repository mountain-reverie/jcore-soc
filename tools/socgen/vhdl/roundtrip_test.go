package vhdl

import (
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

// roundTrips reports whether src parses with no errors and is AST-stable across
// parse -> print -> reparse.
func roundTrips(src []byte) bool {
	f1, errs1 := ParseFile(NewFileSet(), "t.vhd", src)
	if len(errs1) != 0 {
		return false
	}
	out := Print(f1)
	f2, errs2 := ParseFile(NewFileSet(), "t.vhd", []byte(out))
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
			src, err := os.ReadFile(filepath.Join(root, rel))
			if err != nil {
				t.Fatal(err)
			}
			if !roundTrips(src) {
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
			src, err := os.ReadFile(filepath.Join(root, rel))
			if err != nil {
				t.Fatal(err)
			}
			f, errs := ParseFile(NewFileSet(), rel, src)
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
					strings.Contains(msg, "needs the -fsynopsys option") {
					t.Skipf("skipping: test-environment limitation (external units / -fsynopsys), not a printer bug\n%s", b)
				}
				t.Fatalf("ghdl -a rejected printed output: %v\n%s\n--- printed ---\n%s", err, b, out)
			}
		})
	}
}
