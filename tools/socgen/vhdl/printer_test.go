package vhdl

import (
	"strings"
	"testing"
)

func TestPrintInstMapMultiline(t *testing.T) {
	// Reversed input (b before a) so the assertion FALSIFIES accidental sorting:
	// the printer must preserve order (emit, not the printer, sorts).
	src := "architecture a of e is\nbegin\n" +
		"  u : entity work.foo port map (b => y, a => x);\n" +
		"end architecture;"
	f1, errs := ParseFile(NewFileSet(), "t.vhd", []byte(src))
	if errs != nil {
		t.Fatalf("parse: %v", errs)
	}
	out := Print(f1)
	want := "    u : entity work.foo\n" +
		"        port map (\n" +
		"            b => y,\n" +
		"            a => x\n" +
		"        );"
	if !strings.Contains(out, want) {
		t.Errorf("multi-line port map mismatch (printer must NOT sort):\n got:\n%s\nwant block:\n%s", out, want)
	}
	f2, errs2 := ParseFile(NewFileSet(), "t.vhd", []byte(out))
	if errs2 != nil {
		t.Fatalf("reparse: %v", errs2)
	}
	if !equalAST(f1, f2) {
		t.Errorf("round-trip AST changed (printer must preserve association order)")
	}
}

func TestPrintInstMapGenericAndPort(t *testing.T) {
	// An instance with BOTH a generic map and a port map exercises the newline
	// composition between the generic map's `)` and the `port map (` keyword.
	src := "architecture a of e is\nbegin\n" +
		"  u : entity work.foo generic map (W => 32) port map (clk => c);\n" +
		"end architecture;"
	f1, errs := ParseFile(NewFileSet(), "t.vhd", []byte(src))
	if errs != nil {
		t.Fatalf("parse: %v", errs)
	}
	out := Print(f1)
	want := "    u : entity work.foo\n" +
		"        generic map (\n" +
		"            W => 32\n" +
		"        )\n" +
		"        port map (\n" +
		"            clk => c\n" +
		"        );"
	if !strings.Contains(out, want) {
		t.Errorf("generic+port multi-line mismatch:\n got:\n%s\nwant block:\n%s", out, want)
	}
}

func TestPrintInstMapPositional(t *testing.T) {
	// Positional associations (no formal) print actual-only, order preserved.
	src := "architecture a of e is\nbegin\n  u : entity work.foo port map (a, b);\nend architecture;"
	f1, errs := ParseFile(NewFileSet(), "t.vhd", []byte(src))
	if errs != nil {
		t.Fatalf("parse: %v", errs)
	}
	out := Print(f1)
	want := "        port map (\n            a,\n            b\n        );"
	if !strings.Contains(out, want) {
		t.Errorf("positional multi-line mismatch:\n got:\n%s\nwant block:\n%s", out, want)
	}
}

func TestPrintSubprogramSpace(t *testing.T) {
	src := "package p is\n  function f (a : std_logic) return std_logic;\nend package;"
	f1, errs := ParseFile(NewFileSet(), "t.vhd", []byte(src))
	if errs != nil {
		t.Fatalf("parse: %v", errs)
	}
	out := Print(f1)
	if !strings.Contains(out, "function f (a : std_logic) return std_logic") {
		t.Errorf("subprogram should print `f (a …)` with a space before (:\n%s", out)
	}
	f2, errs2 := ParseFile(NewFileSet(), "t.vhd", []byte(out))
	if errs2 != nil {
		t.Fatalf("reparse: %v", errs2)
	}
	if !equalAST(f1, f2) {
		t.Errorf("round-trip AST changed")
	}
}

func TestPrintReparseStable(t *testing.T) {
	srcs := []string{
		"package p is\n  constant C : integer := 5;\nend package;",
		"entity e is\n  port (clk : in std_logic;\n        d : out std_logic_vector(31 downto 0));\nend entity;",
		"package q is\n  type rec_t is record\n    a : std_logic;\n    b : std_logic_vector(7 downto 0);\n  end record;\nend package;",
	}
	for _, s := range srcs {
		f1, errs1 := ParseFile(NewFileSet(), "t.vhd", []byte(s))
		if errs1 != nil {
			t.Fatalf("parse1 %q: %v", s, errs1)
		}
		out := Print(f1)
		f2, errs2 := ParseFile(NewFileSet(), "t.vhd", []byte(out))
		if errs2 != nil {
			t.Fatalf("reparse %q -> %q: %v", s, out, errs2)
		}
		if !equalAST(f1, f2) {
			t.Fatalf("AST changed across round-trip:\nin:  %q\nout: %q", s, out)
		}
	}
}

func TestAttrSpecAlign(t *testing.T) {
	src := "architecture a of e is\n" +
		"  attribute loc : string;\n" +
		"  attribute loc of x : signal is \"p1\";\n" +
		"  attribute loc of pin_long : signal is \"p2\";\n" +
		"begin\nend architecture;"
	f1, errs := ParseFile(NewFileSet(), "t.vhd", []byte(src))
	if errs != nil {
		t.Fatalf("parse: %v", errs)
	}
	out := Print(f1)
	// The two specs' colons align: the shorter "attribute loc of x" pre is padded
	// to the width of "attribute loc of pin_long".
	want := "    attribute loc of x        : signal is \"p1\";\n" +
		"    attribute loc of pin_long : signal is \"p2\";\n"
	if !strings.Contains(out, want) {
		t.Errorf("attribute specs not column-aligned:\n got:\n%s\nwant block:\n%s", out, want)
	}
	// The attribute DECLARATION line is not padded.
	if !strings.Contains(out, "    attribute loc : string;\n") {
		t.Errorf("attribute decl should be unaligned:\n%s", out)
	}
	// Round-trip: whitespace-only, AST stable.
	f2, errs2 := ParseFile(NewFileSet(), "t.vhd", []byte(out))
	if errs2 != nil {
		t.Fatalf("reparse: %v", errs2)
	}
	if !equalAST(f1, f2) {
		t.Errorf("round-trip AST changed")
	}
}
