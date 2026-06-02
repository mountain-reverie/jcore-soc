package vhdl

import "testing"

func TestKeywordLookupCaseInsensitive(t *testing.T) {
	for _, s := range []string{"entity", "ENTITY", "Entity"} {
		if k, ok := LookupKeyword(s); !ok || k != ENTITY {
			t.Fatalf("LookupKeyword(%q) = %v,%v; want ENTITY,true", s, k, ok)
		}
	}
	if _, ok := LookupKeyword("foo"); ok {
		t.Fatalf("LookupKeyword(foo) should not be a keyword")
	}
}

func TestKindString(t *testing.T) {
	if ENTITY.String() != "entity" {
		t.Fatalf("ENTITY.String() = %q", ENTITY.String())
	}
	if IDENT.String() != "IDENT" {
		t.Fatalf("IDENT.String() = %q", IDENT.String())
	}
}

func TestFileSetPosition(t *testing.T) {
	fs := NewFileSet()
	f := fs.AddFile("t.vhd", len("ab\ncd"))
	f.AddLine(3) // line 2 starts at offset 3
	if got := fs.Position(f.Pos(4)); got.Line != 2 || got.Column != 2 {
		t.Fatalf("Pos(4) -> %+v; want line 2 col 2", got)
	}
}
