package vhdl

import "testing"

// kinds runs the lexer to EOF and returns the kind sequence (sans EOF).
func kinds(src string) []Kind {
	b := []byte(src)
	l := NewLexer(b, NewFileSet().AddFile("t.vhd", len(b)))
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
	src := []byte("123 -- hi\n")
	l := NewLexer(src, NewFileSet().AddFile("t.vhd", len(src)))
	if tok := l.Next(); tok.Kind != INT || tok.Lit != "123" {
		t.Fatalf("got %v %q", tok.Kind, tok.Lit)
	}
	if tok := l.Next(); tok.Kind != COMMENT {
		t.Fatalf("want COMMENT got %v", tok.Kind)
	}
}

func TestLexLiterals(t *testing.T) {
	cases := []struct {
		src  string
		kind Kind
		lit  string
	}{
		{`16#FF#`, BASEDLIT, `16#FF#`},
		{`2#1010_1010#`, BASEDLIT, `2#1010_1010#`},
		{`1.5e3`, REAL, `1.5e3`},
		{`3.14`, REAL, `3.14`},
		{`'a'`, CHARLIT, `'a'`},
		{`"hello"`, STRINGLIT, `"hello"`},
		{`x"FF"`, BITSTRINGLIT, `x"FF"`},
		{`b"1010"`, BITSTRINGLIT, `b"1010"`},
		{`\ext id\`, EXTIDENT, `\ext id\`},
	}
	for _, c := range cases {
		b := []byte(c.src)
		l := NewLexer(b, NewFileSet().AddFile("t.vhd", len(b)))
		tok := l.Next()
		if tok.Kind != c.kind || tok.Lit != c.lit {
			t.Fatalf("%q -> %v %q; want %v %q", c.src, tok.Kind, tok.Lit, c.kind, c.lit)
		}
	}
}

func TestLexTickVsChar(t *testing.T) {
	// attribute tick: ident ' ident  => TICK between two idents (not a char literal)
	got := kinds(`clk'event`)
	want := []Kind{IDENT, TICK, IDENT}
	if len(got) != 3 || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
		t.Fatalf("clk'event -> %v", got)
	}
	// char literal in expression context
	if l := NewLexer([]byte(`'0'`), nil); l.Next().Kind != CHARLIT {
		t.Fatalf("'0' should be CHARLIT")
	}
}

func TestLexBrackets(t *testing.T) {
	got := kinds(`[ ]`)
	want := []Kind{LBRACKET, RBRACKET}
	if len(got) != len(want) {
		t.Fatalf("len got=%d want=%d (%v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("tok %d = %v want %v", i, got[i], want[i])
		}
	}
}

func TestLexerPositionMultiLine(t *testing.T) {
	src := []byte("entity foo is\nend entity;")
	fs := NewFileSet()
	f := fs.AddFile("t.vhd", len(src))
	lx := NewLexer(src, f)
	// Scan to the "end" keyword (first token on line 2).
	var endTok Token
	for {
		tok := lx.Next()
		if tok.Kind == EOF {
			t.Fatal("did not find END token")
		}
		if tok.Kind == END {
			endTok = tok
			break
		}
	}
	pos := fs.Position(endTok.Pos)
	if pos.Line != 2 || pos.Column != 1 {
		t.Fatalf("END at %s; want line 2 col 1", pos)
	}
}
