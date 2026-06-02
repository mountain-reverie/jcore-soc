package vhdl

import "testing"

func TestInspectCountsIdents(t *testing.T) {
	df, errs := ParseFile(NewFileSet(), "t.vhd", []byte("package p is\n  constant c : integer := a + b;\nend package;"))
	if len(errs) != 0 {
		t.Fatalf("errs: %v", errs)
	}
	n := 0
	Inspect(df, func(node Node) bool {
		if _, ok := node.(*Ident); ok {
			n++
		}
		return true
	})
	if n != 2 {
		t.Fatalf("expected exactly 2 idents (a, b), got %d", n)
	}
}

func TestInspectPruneStopsDescent(t *testing.T) {
	df, errs := ParseFile(NewFileSet(), "t.vhd", []byte("package p is\n  constant c : integer := a + b;\nend package;"))
	if len(errs) != 0 {
		t.Fatalf("errs: %v", errs)
	}
	idents := 0
	sawPkg := false
	Inspect(df, func(node Node) bool {
		if _, ok := node.(*PackageDecl); ok {
			sawPkg = true
			return false // prune: do not descend into the package's decls
		}
		if _, ok := node.(*Ident); ok {
			idents++
		}
		return true
	})
	if !sawPkg {
		t.Fatal("never visited the PackageDecl")
	}
	if idents != 0 {
		t.Fatalf("pruning failed: descended into package and counted %d idents", idents)
	}
}
