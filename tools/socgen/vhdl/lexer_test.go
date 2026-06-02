package vhdl

import "testing"

// kinds runs the lexer to EOF and returns the kind sequence (sans EOF).
func kinds(src string) []Kind {
	l := NewLexer([]byte(src), "t.vhd")
	var ks []Kind
	for {
		tok := l.Next()
		if tok.Kind == EOF {
			return ks
		}
		ks = append(ks, tok.Kind)
	}
}

func TestLexCore(t *testing.T) {
	got := kinds("entity foo is\n  -- a comment\n  port (clk : in std_logic);\nend entity;")
	want := []Kind{ENTITY, IDENT, IS, PORT, LPAREN, IDENT, COLON, IN, IDENT, RPAREN, SEMICOLON, END, ENTITY, SEMICOLON}
	// NOTE: the comment is emitted as a COMMENT token; filter it for this assertion.
	var f []Kind
	for _, k := range got {
		if k != COMMENT {
			f = append(f, k)
		}
	}
	if len(f) != len(want) {
		t.Fatalf("len got=%d want=%d (%v)", len(f), len(want), f)
	}
	for i := range want {
		if f[i] != want[i] {
			t.Fatalf("tok %d = %v want %v", i, f[i], want[i])
		}
	}
}

func TestLexCompoundDelims(t *testing.T) {
	got := kinds("a <= b; c := d; e => f; x /= y; p <> q")
	want := []Kind{IDENT, LE, IDENT, SEMICOLON, IDENT, ASSIGN, IDENT, SEMICOLON,
		IDENT, ARROW, IDENT, SEMICOLON, IDENT, NE, IDENT, SEMICOLON, IDENT, BOX, IDENT}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("tok %d = %v want %v", i, got[i], want[i])
		}
	}
}

func TestLexIntAndComment(t *testing.T) {
	l := NewLexer([]byte("123 -- hi\n"), "t.vhd")
	if tok := l.Next(); tok.Kind != INT || tok.Lit != "123" {
		t.Fatalf("got %v %q", tok.Kind, tok.Lit)
	}
	if tok := l.Next(); tok.Kind != COMMENT {
		t.Fatalf("want COMMENT got %v", tok.Kind)
	}
}
