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
	if n < 2 {
		t.Fatalf("expected >=2 idents (a, b), got %d", n)
	}
}
