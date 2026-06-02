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
