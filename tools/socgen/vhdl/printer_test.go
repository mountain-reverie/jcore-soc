package vhdl

import (
	"strings"
	"testing"
)

func TestPrintInstMapMultiline(t *testing.T) {
	src := "architecture a of e is\nbegin\n" +
		"  u : entity work.foo port map (a => x, b => y);\n" +
		"end architecture;"
	f1, errs := ParseFile(NewFileSet(), "t.vhd", []byte(src))
	if errs != nil {
		t.Fatalf("parse: %v", errs)
	}
	out := Print(f1)
	want := "    u : entity work.foo\n" +
		"        port map (\n" +
		"            a => x,\n" +
		"            b => y\n" +
		"        );"
	if !strings.Contains(out, want) {
		t.Errorf("multi-line port map mismatch:\n got:\n%s\nwant block:\n%s", out, want)
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
