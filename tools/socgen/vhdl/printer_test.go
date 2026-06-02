package vhdl

import "testing"

func TestPrintReparseStable(t *testing.T) {
	srcs := []string{
		"package p is\n  constant C : integer := 5;\nend package;",
		"entity e is\n  port (clk : in std_logic;\n        d : out std_logic_vector(31 downto 0));\nend entity;",
		"package q is\n  type rec_t is record\n    a : std_logic;\n    b : std_logic_vector(7 downto 0);\n  end record;\nend package;",
	}
	for _, s := range srcs {
		f1, errs1 := ParseFile(NewFileSet(), "t.vhd", []byte(s))
		if len(errs1) != 0 {
			t.Fatalf("parse1 %q: %v", s, errs1)
		}
		out := Print(f1)
		f2, errs2 := ParseFile(NewFileSet(), "t.vhd", []byte(out))
		if len(errs2) != 0 {
			t.Fatalf("reparse %q -> %q: %v", s, out, errs2)
		}
		if !equalAST(f1, f2) {
			t.Fatalf("AST changed across round-trip:\nin:  %q\nout: %q", s, out)
		}
	}
}
