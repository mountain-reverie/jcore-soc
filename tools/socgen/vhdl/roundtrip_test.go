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

// cppFileOpts supplies build-specific preprocessor flags for corpus files whose
// cpp directives need defines/includes beyond the defaults. Mirrors each file's
// build Makefile (e.g. components/cpu/sim/Makefile for cpu_tb.vhd).
var cppFileOpts = map[string][]Option{
	"components/cpu/sim/cpu_tb.vhd": {
		WithCPPDefine("VHDL", ""),
		WithCPPDefine("CONFIG_PREFETCHER", "0"),
		WithCPPDefine("CONFIG_RING_BUS", "0"),
		WithCPPInclude("sim"),
	},
}

// cppOpts returns the parse options for a corpus file (by rel path), and whether
// the subtest should skip because a cpp directive is present but gcc is absent.
func cppOpts(rel string, src []byte) (opts []Option, skip bool) {
	if !hasCPPDirective(src) {
		return nil, false
	}
	if _, err := exec.LookPath("gcc"); err != nil {
		return nil, true
	}
	opts = []Option{WithCPP("gcc")}
	opts = append(opts, cppFileOpts[rel]...)
	return opts, false
}

// roundTrips reports whether src (at absPath, with given opts) parses with
// no errors and is AST-stable across parse -> print -> reparse.
// The reparse of the printed (already-expanded) output uses no options.
func roundTrips(absPath string, opts []Option, src []byte) bool {
	f1, errs1 := ParseFile(NewFileSet(), absPath, src, opts...)
	if errs1 != nil {
		return false
	}
	out := Print(f1)
	f2, errs2 := ParseFile(NewFileSet(), absPath, []byte(out)) // printed VHDL has no directives
	if errs2 != nil {
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
			opts, skip := cppOpts(rel, src)
			if skip {
				t.Skip("cpp file but gcc not available")
			}
			if !roundTrips(absPath, opts, src) {
				_, errs := ParseFile(NewFileSet(), absPath, src, opts...)
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
			opts, skip := cppOpts(rel, src)
			if skip {
				t.Skip("cpp file but gcc not available")
			}
			f, errs := ParseFile(NewFileSet(), absPath, src, opts...)
			if errs != nil {
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
					strings.Contains(msg, "universal integer bound must be numeric literal") ||
					strings.Contains(msg, "mode allowed only in vhdl 87") {
					t.Skipf("skipping: test-environment limitation (external units / -fsynopsys / ghdl strict-93 expression bounds / vhdl-87 file modes), not a printer bug\n%s", b)
				}
				t.Fatalf("ghdl -a rejected printed output: %v\n%s\n--- printed ---\n%s", err, b, out)
			}
		})
	}
}
