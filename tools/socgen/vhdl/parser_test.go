package vhdl

import (
	"strings"
	"testing"
)

func mustParseExpr(t *testing.T, src string) Expr {
	t.Helper()
	p := newParserFromExpr([]byte(src))
	e := p.parseExpr()
	if len(p.errs) != 0 {
		t.Fatalf("errs: %v", p.errs)
	}
	return e
}

func TestParseMinimalExpr(t *testing.T) {
	if r, ok := mustParseExpr(t, "31 downto 0").(*Range); !ok || r.Dir != DOWNTO {
		t.Fatalf("range: %#v", r)
	}
	if c, ok := mustParseExpr(t, "f(a, b)").(*CallExpr); !ok || len(c.Args) != 2 {
		t.Fatalf("call: %#v", c)
	}
	if _, ok := mustParseExpr(t, "a + b").(*BinaryExpr); !ok {
		t.Fatalf("binary")
	}
	if _, ok := mustParseExpr(t, "(others => '0')").(*Aggregate); !ok {
		t.Fatalf("paren/aggregate")
	}
}

func TestParenVsAggregate(t *testing.T) {
	if _, ok := mustParseExpr(t, "(a + b)").(*ParenExpr); !ok {
		t.Fatal("expected ParenExpr for single positional element")
	}
	ag, ok := mustParseExpr(t, "(others => '0')").(*Aggregate)
	if !ok || len(ag.Elems) != 1 || ag.Elems[0].Choices == nil {
		t.Fatalf("expected 1-element named aggregate: %#v", ag)
	}
	ag2, ok := mustParseExpr(t, "(1, 2, 3)").(*Aggregate)
	if !ok || len(ag2.Elems) != 3 || ag2.Elems[0].Choices != nil {
		t.Fatalf("expected 3-element positional aggregate: %#v", ag2)
	}
	// named with choice
	ag3, ok := mustParseExpr(t, "(0 => '1', others => '0')").(*Aggregate)
	if !ok || len(ag3.Elems) != 2 {
		t.Fatalf("expected 2-element aggregate: %#v", ag3)
	}
	// multi-choice with '|'
	ag4, ok := mustParseExpr(t, "(1 | 2 => x)").(*Aggregate)
	if !ok || len(ag4.Elems) != 1 || len(ag4.Elems[0].Choices) != 2 {
		t.Fatalf("expected 1-element aggregate with 2 choices: %#v", ag4)
	}
	// discrete-range choice
	ag5, ok := mustParseExpr(t, "(1 to 3 => x)").(*Aggregate)
	if !ok || len(ag5.Elems) != 1 {
		t.Fatalf("expected 1-element aggregate with range choice: %#v", ag5)
	}
	if _, ok := ag5.Elems[0].Choices[0].(*Range); !ok {
		t.Fatalf("expected range choice, got %#v", ag5.Elems[0].Choices[0])
	}
}

func TestExprTokenTypedOps(t *testing.T) {
	be, ok := mustParseExpr(t, "a + b").(*BinaryExpr)
	if !ok || be.Op != PLUS {
		t.Fatalf("binary op: %#v", be)
	}
	id, ok := be.X.(*Ident)
	if !ok || id.Name != "a" {
		t.Fatalf("lhs ident: %#v", be.X)
	}
	if got := id.End() - id.Pos(); got != Pos(len("a")) {
		t.Fatalf("Ident.End-Pos = %d; want 1", got)
	}
}

func TestExprPrecedence(t *testing.T) {
	// a + b * c  =>  a + (b*c): top is + with right-child *
	e := mustParseExpr(t, "a + b * c")
	be, ok := e.(*BinaryExpr)
	if !ok || be.Op != PLUS {
		t.Fatalf("top: %#v", e)
	}
	r, ok := be.Y.(*BinaryExpr)
	if !ok || r.Op != STAR {
		t.Fatalf("right: %#v", be.Y)
	}
}

func TestExprLeftAssoc(t *testing.T) {
	// a - b - c  =>  (a-b)-c: top is - with left-child -
	be, ok := mustParseExpr(t, "a - b - c").(*BinaryExpr)
	if !ok || be.Op != MINUS {
		t.Fatalf("top: %#v", be)
	}
	if l, ok := be.X.(*BinaryExpr); !ok || l.Op != MINUS {
		t.Fatalf("left: %#v", be.X)
	}
}

func TestExprUnary(t *testing.T) {
	if u, ok := mustParseExpr(t, "not a").(*UnaryExpr); !ok || u.Op != NOT {
		t.Fatalf("not: %#v", u)
	}
	if u, ok := mustParseExpr(t, "-b").(*UnaryExpr); !ok || u.Op != MINUS {
		t.Fatalf("neg: %#v", u)
	}
	if u, ok := mustParseExpr(t, "+b").(*UnaryExpr); !ok || u.Op != PLUS {
		t.Fatalf("pos: %#v", u)
	}
	if u, ok := mustParseExpr(t, "abs x").(*UnaryExpr); !ok || u.Op != ABS {
		t.Fatalf("abs: %#v", u)
	}
}

func TestExprPow(t *testing.T) {
	if b, ok := mustParseExpr(t, "2 ** n").(*BinaryExpr); !ok || b.Op != EXP {
		t.Fatalf("pow: %#v", b)
	}
}

func TestExprRelationalAndLogical(t *testing.T) {
	// a = b and c  =>  (a=b) and c : top AND, left is relational =
	be, ok := mustParseExpr(t, "a = b and c").(*BinaryExpr)
	if !ok || be.Op != AND {
		t.Fatalf("top: %#v", be)
	}
	if l, ok := be.X.(*BinaryExpr); !ok || l.Op != EQ {
		t.Fatalf("left: %#v", be.X)
	}
}

func parse(t *testing.T, src string) (*DesignFile, []error) {
	t.Helper()
	return ParseFile(NewFileSet(), "t.vhd", []byte(src))
}

func parseDecls(t *testing.T, src string) []Decl {
	t.Helper()
	df, errs := parse(t, "package p is\n"+src+"\nend package;")
	if len(errs) != 0 {
		t.Fatalf("errs: %v", errs)
	}
	return df.Units[0].(*PackageDecl).Decls
}

func TestParseDecls(t *testing.T) {
	ds := parseDecls(t, `
		constant C : integer := 5;
		type state_t is (IDLE, RUN);
		type rec_t is record
		  a : std_logic;
		  b : std_logic_vector(31 downto 0);
		end record;
		component comp is
		  port (clk : in std_logic; q : out std_logic);
		end component;`)
	if len(ds) != 4 {
		t.Fatalf("got %d decls", len(ds))
	}
	if c, ok := ds[0].(*ConstantDecl); !ok || c.Names[0] != "C" {
		t.Fatal("const")
	}
	if td, ok := ds[1].(*TypeDecl); !ok {
		t.Fatal("type")
	} else if _, ok := td.Def.(*EnumDef); !ok {
		t.Fatal("enum")
	}
	if td, ok := ds[2].(*TypeDecl); !ok {
		t.Fatal("rec type")
	} else if r, ok := td.Def.(*RecordDef); !ok || len(r.Fields) != 2 {
		t.Fatal("record fields")
	}
	if cm, ok := ds[3].(*ComponentDecl); !ok || len(cm.Ports) != 2 {
		t.Fatal("component")
	}
}

func TestParseEntity(t *testing.T) {
	df, errs := parse(t, `
entity cpu is
  generic (W : integer := 32);
  port (
    clk : in  std_logic;
    d   : out std_logic_vector(W-1 downto 0));
end entity cpu;`)
	if len(errs) != 0 {
		t.Fatalf("errs: %v", errs)
	}
	e := df.Units[0].(*EntityDecl)
	if e.Name != "cpu" || len(e.Generics) != 1 || len(e.Ports) != 2 {
		t.Fatalf("%#v", e)
	}
	if e.Ports[1].Mode != "out" || e.Ports[1].SubtypeMark != "std_logic_vector" {
		t.Fatal("port shape")
	}
}

func TestQualifiedExpr(t *testing.T) {
	q, ok := mustParseExpr(t, "std_logic_vector'(others => '0')").(*QualifiedExpr)
	if !ok {
		t.Fatalf("expected QualifiedExpr, got %#v", q)
	}
	if _, ok := q.X.(*Aggregate); !ok {
		t.Fatalf("expected aggregate operand, got %#v", q.X)
	}
	q2, ok := mustParseExpr(t, "integer'(5)").(*QualifiedExpr)
	if !ok {
		t.Fatalf("expected QualifiedExpr, got %#v", q2)
	}
	if _, ok := q2.X.(*ParenExpr); !ok {
		t.Fatalf("expected paren operand, got %#v", q2.X)
	}
	// attribute tick must STILL parse as a (flattened) Ident, not a qualified expr
	if id, ok := mustParseExpr(t, "x'high").(*Ident); !ok || id.Name != "x'high" {
		t.Fatalf("expected Ident x'high, got %#v", mustParseExpr(t, "x'high"))
	}

	// round-trip: print the parsed qualified expr and reparse to an equal AST.
	src := "std_logic_vector'(others => '0')"
	q3 := mustParseExpr(t, src)
	var b strings.Builder
	printExpr(&b, q3)
	if out := b.String(); out != src {
		t.Fatalf("print mismatch: got %q want %q", out, src)
	}
	if !equalAST(q3, mustParseExpr(t, b.String())) {
		t.Fatal("qualified expr not AST-stable across print/reparse")
	}
}

func TestParseSubprogramDecls(t *testing.T) {
	ds := parseDecls(t, `
		function to_slv(b : std_logic; s : integer) return std_logic_vector;
		procedure read(M : inout ram_t; P : in integer);
		pure function f return integer;
		impure function g(x : integer) return bit;`)
	if len(ds) != 4 {
		t.Fatalf("got %d decls", len(ds))
	}
	f0, ok := ds[0].(*SubprogramDecl)
	if !ok || f0.IsProcedure || f0.Designator != "to_slv" || len(f0.Params) != 2 || f0.ReturnMark != "std_logic_vector" {
		t.Fatalf("func0: %#v", ds[0])
	}
	p1, ok := ds[1].(*SubprogramDecl)
	if !ok || !p1.IsProcedure || p1.Designator != "read" || len(p1.Params) != 2 {
		t.Fatalf("proc1: %#v", ds[1])
	}
	if f2, ok := ds[2].(*SubprogramDecl); !ok || !f2.Pure || len(f2.Params) != 0 || f2.ReturnMark != "integer" {
		t.Fatalf("pure func2: %#v", ds[2])
	}
	if f3, ok := ds[3].(*SubprogramDecl); !ok || !f3.Impure || f3.Designator != "g" {
		t.Fatalf("impure func3: %#v", ds[3])
	}

	// operator-symbol designator (string literal, quotes retained)
	ds2 := parseDecls(t, `function "+"(a : bit; b : bit) return bit;`)
	op, ok := ds2[0].(*SubprogramDecl)
	if !ok || op.Designator != `"+"` {
		t.Fatalf("operator designator: %#v", ds2[0])
	}

	// round-trip: the parsed decls must survive print->reparse.
	df, errs := ParseFile(NewFileSet(), "t.vhd", []byte("package p is\nfunction to_slv(b : std_logic; s : integer) return std_logic_vector;\nend package;"))
	if len(errs) != 0 {
		t.Fatalf("errs: %v", errs)
	}
	out := Print(df)
	df2, errs2 := ParseFile(NewFileSet(), "t.vhd", []byte(out))
	if len(errs2) != 0 {
		t.Fatalf("reparse errs: %v\n%s", errs2, out)
	}
	if !equalAST(df, df2) {
		t.Fatalf("subprogram decl not AST-stable:\n%s", out)
	}
}

func TestParseAttributeDecls(t *testing.T) {
	ds := parseDecls(t, `
		attribute num_words : natural;
		attribute num_words of reg8x4_data : subtype is 4;
		attribute keep of a, b : signal is true;`)
	if len(ds) != 3 {
		t.Fatalf("got %d decls", len(ds))
	}
	d0, ok := ds[0].(*AttributeDecl)
	if !ok || d0.Name != "num_words" || d0.TypeMark != "natural" {
		t.Fatalf("attr decl: %#v", ds[0])
	}
	s1, ok := ds[1].(*AttributeSpec)
	if !ok || s1.Name != "num_words" || len(s1.Entities) != 1 || s1.Entities[0] != "reg8x4_data" || s1.EntityClass != SUBTYPE {
		t.Fatalf("attr spec: %#v", ds[1])
	}
	s2, ok := ds[2].(*AttributeSpec)
	if !ok || len(s2.Entities) != 2 || s2.EntityClass != SIGNAL {
		t.Fatalf("attr spec 2: %#v", ds[2])
	}
	// round-trip
	df, errs := ParseFile(NewFileSet(), "t.vhd", []byte("package p is\nattribute num_words of reg8x4_data : subtype is 4;\nend package;"))
	if len(errs) != 0 {
		t.Fatalf("errs: %v", errs)
	}
	out := Print(df)
	df2, errs2 := ParseFile(NewFileSet(), "t.vhd", []byte(out))
	if len(errs2) != 0 || !equalAST(df, df2) {
		t.Fatalf("attr spec not AST-stable: errs=%v\n%s", errs2, out)
	}
}

func TestParseAliasDecls(t *testing.T) {
	ds := parseDecls(t, `
		alias slv is std_logic_vector;
		alias destv : std_logic_vector(7 downto 0) is dest;`)
	if len(ds) != 2 {
		t.Fatalf("got %d decls", len(ds))
	}
	a0, ok := ds[0].(*AliasDecl)
	if !ok || a0.Name != "slv" || a0.SubtypeMark != "" {
		t.Fatalf("alias0: %#v", ds[0])
	}
	a1, ok := ds[1].(*AliasDecl)
	if !ok || a1.Name != "destv" || a1.SubtypeMark != "std_logic_vector" || a1.Target == nil {
		t.Fatalf("alias1: %#v", ds[1])
	}
	// round-trip
	df, errs := ParseFile(NewFileSet(), "t.vhd", []byte("package p is\nalias destv : std_logic_vector(7 downto 0) is dest;\nend package;"))
	if len(errs) != 0 {
		t.Fatalf("errs: %v", errs)
	}
	out := Print(df)
	df2, errs2 := ParseFile(NewFileSet(), "t.vhd", []byte(out))
	if len(errs2) != 0 || !equalAST(df, df2) {
		t.Fatalf("alias not AST-stable: errs=%v\n%s", errs2, out)
	}
}

func TestParseGroupDecls(t *testing.T) {
	ds := parseDecls(t, `
		group local_ports is (signal <>);
		group sigs : global_ports(rx, tx);`)
	if len(ds) != 2 {
		t.Fatalf("got %d decls", len(ds))
	}
	tmpl, ok := ds[0].(*GroupTemplateDecl)
	if !ok || tmpl.Name != "local_ports" || len(tmpl.Classes) != 1 || tmpl.Classes[0] != "signal <>" {
		t.Fatalf("group template: %#v", ds[0])
	}
	g, ok := ds[1].(*GroupDecl)
	if !ok || g.Name != "sigs" || g.TemplateMark != "global_ports" || len(g.Constituents) != 2 {
		t.Fatalf("group decl: %#v", ds[1])
	}
	// canonical-casing + multi-class: uppercase source keyword must normalize.
	ds2 := parseDecls(t, `group g2 is (SIGNAL <>, label);`)
	tmpl2, ok := ds2[0].(*GroupTemplateDecl)
	if !ok || len(tmpl2.Classes) != 2 || tmpl2.Classes[0] != "signal <>" || tmpl2.Classes[1] != "label" {
		t.Fatalf("canonical class casing/multi-class: %#v", ds2[0])
	}
	// round-trip
	df, errs := ParseFile(NewFileSet(), "t.vhd", []byte("package p is\ngroup local_ports is (signal <>);\ngroup sigs : global_ports(rx, tx);\nend package;"))
	if len(errs) != 0 {
		t.Fatalf("errs: %v", errs)
	}
	out := Print(df)
	df2, errs2 := ParseFile(NewFileSet(), "t.vhd", []byte(out))
	if len(errs2) != 0 || !equalAST(df, df2) {
		t.Fatalf("group not AST-stable: errs=%v\n%s", errs2, out)
	}
}

func TestDeferredUnitTagged(t *testing.T) {
	_, errs := parse(t, "architecture a of e is\nbegin\nend architecture;")
	if len(errs) == 0 {
		t.Fatal("expected a deferred-unit error")
	}
	found := false
	for _, e := range errs {
		if strings.Contains(e.Error(), "deferred") {
			found = true
		}
	}
	if !found {
		t.Fatalf("want a deferred-unit error, got %v", errs)
	}
}

func TestParseEntityDeclarativePart(t *testing.T) {
	src := `entity e is
  port (clk : in std_logic);
  type ram_t is array(7 downto 0) of std_logic;
  attribute ram_style of clk : signal is "block";
  subtype word_t is std_logic;
end entity e;`
	df, errs := ParseFile(NewFileSet(), "t.vhd", []byte(src))
	if len(errs) != 0 {
		t.Fatalf("errs: %v", errs)
	}
	e := df.Units[0].(*EntityDecl)
	if len(e.Ports) != 1 {
		t.Fatalf("ports: %#v", e)
	}
	if len(e.Decls) != 3 {
		t.Fatalf("entity decls: got %d: %#v", len(e.Decls), e.Decls)
	}
	// round-trip
	out := Print(df)
	df2, errs2 := ParseFile(NewFileSet(), "t.vhd", []byte(out))
	if len(errs2) != 0 || !equalAST(df, df2) {
		t.Fatalf("entity declarative part not AST-stable: errs=%v\n%s", errs2, out)
	}
}

func TestEntityStatementPartDeferred(t *testing.T) {
	// An entity with a passive statement part (begin) is deferred (errors).
	_, errs := ParseFile(NewFileSet(), "t.vhd", []byte("entity e is\nbegin\n  assert true;\nend entity;"))
	if len(errs) == 0 {
		t.Fatal("expected a deferred error for entity statement part")
	}
	found := false
	for _, e := range errs {
		if strings.Contains(e.Error(), "deferred") {
			found = true
		}
	}
	if !found {
		t.Fatalf("want a 'deferred' error, got %v", errs)
	}
}
