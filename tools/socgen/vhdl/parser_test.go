package vhdl

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/j-core/jcore-soc/tools/socgen/internal/errutil"
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

func parse(t *testing.T, src string) (*DesignFile, error) {
	t.Helper()
	return ParseFile(NewFileSet(), "t.vhd", []byte(src))
}

func parseDecls(t *testing.T, src string) []Decl {
	t.Helper()
	df, errs := parse(t, "package p is\n"+src+"\nend package;")
	if errs != nil {
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
	if errs != nil {
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
	if errs != nil {
		t.Fatalf("errs: %v", errs)
	}
	out := Print(df)
	df2, errs2 := ParseFile(NewFileSet(), "t.vhd", []byte(out))
	if errs2 != nil {
		t.Fatalf("reparse errs: %v\n%s", errs2, out)
	}
	if !equalAST(df, df2) {
		t.Fatalf("subprogram decl not AST-stable:\n%s", out)
	}
}

func TestParseExtendedIdDesignator(t *testing.T) {
	// Extended-identifier designator: function \?=\ (...) return ...;
	const src = "package p is\nfunction \\?=\\ (l, r : bit) return bit;\nend package;"
	df, errs := ParseFile(NewFileSet(), "t.vhd", []byte(src))
	if errs != nil {
		t.Fatalf("parse errors: %v", errs)
	}
	pkg, ok := df.Units[0].(*PackageDecl)
	if !ok || len(pkg.Decls) != 1 {
		t.Fatalf("expected PackageDecl with 1 decl, got %T len=%d", df.Units[0], len(pkg.Decls))
	}
	sd, ok := pkg.Decls[0].(*SubprogramDecl)
	if !ok {
		t.Fatalf("expected *SubprogramDecl, got %T", pkg.Decls[0])
	}
	if sd.Designator == "" {
		t.Fatal("Designator is empty")
	}
	t.Logf("EXTIDENT.Lit for \\?=\\ = %q", sd.Designator)

	// Round-trip: print → reparse → equalAST
	out := Print(df)
	df2, errs2 := ParseFile(NewFileSet(), "t.vhd", []byte(out))
	if errs2 != nil {
		t.Fatalf("reparse errors: %v\n--- printed ---\n%s", errs2, out)
	}
	if !equalAST(df, df2) {
		t.Fatalf("extended-id designator not AST-stable:\n%s", out)
	}
	sd2, ok := df2.Units[0].(*PackageDecl).Decls[0].(*SubprogramDecl)
	if !ok || sd2.Designator != sd.Designator {
		t.Fatalf("round-trip changed designator: %q → %q", sd.Designator, sd2.Designator)
	}

	// Regression: operator-symbol (STRINGLIT) still works.
	ds2 := parseDecls(t, `function "+"(a : bit; b : bit) return bit;`)
	op, ok := ds2[0].(*SubprogramDecl)
	if !ok || op.Designator != `"+"` {
		t.Fatalf("operator-symbol regression: %#v", ds2[0])
	}

	// Regression: plain identifier still works.
	ds3 := parseDecls(t, `function foo(a : bit) return bit;`)
	fn, ok := ds3[0].(*SubprogramDecl)
	if !ok || fn.Designator != "foo" {
		t.Fatalf("ident regression: %#v", ds3[0])
	}
}

// TestParseExtendedIdDesignatorBody checks that a subprogram BODY with an
// extended-identifier designator AND an extended-identifier closing label
// parses without errors and round-trips (parse→print→reparse→equalAST).
// Also confirms STRINGLIT and IDENT closing labels still work.
func TestParseExtendedIdDesignatorBody(t *testing.T) {
	// Extended-id designator + extended-id closing label.
	const src = `package body p is
  function \?=\ (l, r : bit) return bit is
  begin
    return l;
  end function \?=\;
end package body;`
	df, errs := ParseFile(NewFileSet(), "t.vhd", []byte(src))
	if errs != nil {
		t.Fatalf("parse errors: %v", errs)
	}
	// Round-trip.
	out := Print(df)
	df2, errs2 := ParseFile(NewFileSet(), "t.vhd", []byte(out))
	if errs2 != nil {
		t.Fatalf("reparse errors: %v\n--- printed ---\n%s", errs2, out)
	}
	if !equalAST(df, df2) {
		t.Fatalf("extended-id body not AST-stable:\n%s", out)
	}

	// Regression: STRINGLIT closing label still works.
	const src2 = `package body p is
  function "+" (a, b : bit) return bit is
  begin
    return a;
  end function "+";
end package body;`
	df3, errs3 := ParseFile(NewFileSet(), "t.vhd", []byte(src2))
	if errs3 != nil {
		t.Fatalf("stringlit-closing-label parse errors: %v", errs3)
	}
	out3 := Print(df3)
	df4, errs4 := ParseFile(NewFileSet(), "t.vhd", []byte(out3))
	if errs4 != nil || !equalAST(df3, df4) {
		t.Fatalf("stringlit-closing-label not AST-stable:\n%s", out3)
	}

	// Regression: plain IDENT closing label still works.
	const src3 = `package body p is
  function f (a : bit) return bit is
  begin
    return a;
  end function f;
end package body;`
	df5, errs5 := ParseFile(NewFileSet(), "t.vhd", []byte(src3))
	if errs5 != nil {
		t.Fatalf("ident-closing-label parse errors: %v", errs5)
	}
	out5 := Print(df5)
	df6, errs6 := ParseFile(NewFileSet(), "t.vhd", []byte(out5))
	if errs6 != nil || !equalAST(df5, df6) {
		t.Fatalf("ident-closing-label not AST-stable:\n%s", out5)
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
	if errs != nil {
		t.Fatalf("errs: %v", errs)
	}
	out := Print(df)
	df2, errs2 := ParseFile(NewFileSet(), "t.vhd", []byte(out))
	if errs2 != nil || !equalAST(df, df2) {
		t.Fatalf("attr spec not AST-stable: errs=%v\n%s", errs2, out)
	}

	// malformed entity class (not a reserved word) must be rejected.
	_, errs3 := ParseFile(NewFileSet(), "t.vhd", []byte("package p is\nattribute foo of x : notakeyword is 1;\nend package;"))
	if errs3 == nil {
		t.Fatal("expected an error for non-keyword entity class")
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
	if errs != nil {
		t.Fatalf("errs: %v", errs)
	}
	out := Print(df)
	df2, errs2 := ParseFile(NewFileSet(), "t.vhd", []byte(out))
	if errs2 != nil || !equalAST(df, df2) {
		t.Fatalf("alias not AST-stable: errs=%v\n%s", errs2, out)
	}
}

func TestParseAliasSignature(t *testing.T) {
	roundTrip := func(t *testing.T, src string) *AliasDecl {
		t.Helper()
		full := "package p is\n" + src + "\nend package;"
		df, errs := ParseFile(NewFileSet(), "t.vhd", []byte(full))
		if errs != nil {
			t.Fatalf("parse errors: %v", errs)
		}
		pkg := df.Units[0].(*PackageDecl)
		a, ok := pkg.Decls[0].(*AliasDecl)
		if !ok {
			t.Fatalf("expected *AliasDecl, got %T", pkg.Decls[0])
		}
		// round-trip
		out := Print(df)
		df2, errs2 := ParseFile(NewFileSet(), "t.vhd", []byte(out))
		if errs2 != nil || !equalAST(df, df2) {
			t.Fatalf("not AST-stable: errs=%v\n%s", errs2, out)
		}
		return a
	}

	t.Run("return_sig", func(t *testing.T) {
		a := roundTrip(t, `alias to_slv2 is to_slv [unresolved_ufixed return std_logic_vector];`)
		if a.Signature == nil {
			t.Fatal("expected Signature != nil")
		}
		if len(a.Signature.Types) != 1 || a.Signature.Types[0] != "unresolved_ufixed" {
			t.Fatalf("Types = %v", a.Signature.Types)
		}
		if a.Signature.Return != "std_logic_vector" {
			t.Fatalf("Return = %q", a.Signature.Return)
		}
	})

	t.Run("multi_type_no_return", func(t *testing.T) {
		a := roundTrip(t, `alias bwrite is write [line, unresolved_ufixed, side, width];`)
		if a.Signature == nil {
			t.Fatal("expected Signature != nil")
		}
		if len(a.Signature.Types) != 4 {
			t.Fatalf("Types len = %d, want 4: %v", len(a.Signature.Types), a.Signature.Types)
		}
		if a.Signature.Return != "" {
			t.Fatalf("Return = %q, want empty", a.Signature.Return)
		}
	})

	t.Run("two_types_no_return", func(t *testing.T) {
		a := roundTrip(t, `alias bread is read [line, unresolved_ufixed];`)
		if a.Signature == nil {
			t.Fatal("expected Signature != nil")
		}
		if len(a.Signature.Types) != 2 {
			t.Fatalf("Types len = %d, want 2: %v", len(a.Signature.Types), a.Signature.Types)
		}
		if a.Signature.Return != "" {
			t.Fatalf("Return = %q, want empty", a.Signature.Return)
		}
	})

	t.Run("no_signature", func(t *testing.T) {
		a := roundTrip(t, `alias y is x;`)
		if a.Signature != nil {
			t.Fatalf("expected Signature == nil, got %+v", a.Signature)
		}
	})
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
	if errs != nil {
		t.Fatalf("errs: %v", errs)
	}
	out := Print(df)
	df2, errs2 := ParseFile(NewFileSet(), "t.vhd", []byte(out))
	if errs2 != nil || !equalAST(df, df2) {
		t.Fatalf("group not AST-stable: errs=%v\n%s", errs2, out)
	}
}

func TestDeferredUnitTagged(t *testing.T) {
	// A block statement inside an architecture is not yet parsed (deferred).
	// Verify that parsing such a file yields at least one error containing
	// "deferred", exercising the deferral-tagging mechanism.
	src := "architecture a of e is begin b: block begin end block; end architecture;"
	_, err := ParseFile(NewFileSet(), "t.vhd", []byte(src))
	if err == nil {
		t.Fatalf("expected deferral errors for block statement, got none")
	}
	found := false
	for _, e := range errutil.Errors(err) {
		var pe *ParseError
		if errors.As(e, &pe) && strings.Contains(pe.Msg, "deferred") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected an error containing \"deferred\", got: %v", err)
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
	if errs != nil {
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
	if errs2 != nil || !equalAST(df, df2) {
		t.Fatalf("entity declarative part not AST-stable: errs=%v\n%s", errs2, out)
	}
}

func TestParseArchitectureSimple(t *testing.T) {
	src := `architecture rtl of e is
  signal x : std_logic;
begin
  x <= a;
  y <= a and b;
end architecture rtl;`
	df, errs := ParseFile(NewFileSet(), "t.vhd", []byte(src))
	if errs != nil {
		t.Fatalf("errs: %v", errs)
	}
	arch, ok := df.Units[0].(*ArchitectureBody)
	if !ok || arch.Name != "rtl" || arch.Entity != "e" {
		t.Fatalf("arch: %#v", df.Units[0])
	}
	if len(arch.Decls) != 1 {
		t.Fatalf("decls: %d", len(arch.Decls))
	}
	if len(arch.Stmts) != 2 {
		t.Fatalf("stmts: %d", len(arch.Stmts))
	}
	if _, ok := arch.Stmts[0].(*ConcurrentSignalAssign); !ok {
		t.Fatalf("stmt0: %#v", arch.Stmts[0])
	}
	// round-trip
	out := Print(df)
	df2, errs2 := ParseFile(NewFileSet(), "t.vhd", []byte(out))
	if errs2 != nil || !equalAST(df, df2) {
		t.Fatalf("architecture not AST-stable: errs=%v\n%s", errs2, out)
	}
}


func TestParseConditionalAssign(t *testing.T) {
	src := `architecture rtl of e is
begin
  q <= a when sel = '1' else b;
  r <= x when c1 else y when c2 else z;
  s <= d;
  clk <= '0' after 5 ns when rst = '1' else '1';   -- conditional arm WITH after
end architecture;`
	df, errs := ParseFile(NewFileSet(), "t.vhd", []byte(src))
	if errs != nil {
		t.Fatalf("errs: %v", errs)
	}
	st := df.Units[0].(*ArchitectureBody).Stmts
	q := st[0].(*ConcurrentSignalAssign)
	if q.Waveform != nil || len(q.Conds) != 2 {
		t.Fatalf("q conds: %#v", q)
	}
	if q.Conds[0].Cond == nil || q.Conds[1].Cond != nil { // last arm is the bare else
		t.Fatalf("q cond shape: %#v", q.Conds)
	}
	if len(q.Conds[0].Waveform) != 1 {
		t.Fatalf("q arm0 waveform: %#v", q.Conds[0])
	}
	r := st[1].(*ConcurrentSignalAssign)
	if len(r.Conds) != 3 {
		t.Fatalf("r conds: %d", len(r.Conds))
	}
	s := st[2].(*ConcurrentSignalAssign)
	if s.Waveform == nil || len(s.Conds) != 0 { // simple assignment unchanged
		t.Fatalf("s simple: %#v", s)
	}
	clk := st[3].(*ConcurrentSignalAssign)
	if len(clk.Conds) != 2 || clk.Conds[0].Waveform[0].After == nil {
		t.Fatalf("conditional arm with after: %#v", clk.Conds[0])
	}
	// round-trip
	out := Print(df)
	df2, errs2 := ParseFile(NewFileSet(), "t.vhd", []byte(out))
	if errs2 != nil || !equalAST(df, df2) {
		t.Fatalf("conditional assign not AST-stable: errs=%v\n%s", errs2, out)
	}
}

func TestParseInstantiation(t *testing.T) {
	src := `architecture rtl of e is
begin
  u0 : entity work.cpu generic map (W => 32) port map (clk => clk, q => open);
  u1 : entity work.foo port map (a, b, c);
  u2 : entity work.bar(rtl) port map (clk => clk);
  c0 : comp_x port map (x => y);
  c1 : component comp_y port map (x => y);
end architecture;`
	df, errs := ParseFile(NewFileSet(), "t.vhd", []byte(src))
	if errs != nil {
		t.Fatalf("errs: %v", errs)
	}
	arch := df.Units[0].(*ArchitectureBody)
	if len(arch.Stmts) != 5 {
		t.Fatalf("stmts: %d", len(arch.Stmts))
	}
	u0, ok := arch.Stmts[0].(*InstantiationStmt)
	if !ok || u0.Label != "u0" || u0.UnitKind != ENTITY || u0.Unit != "work.cpu" || len(u0.GenericMap) != 1 || len(u0.PortMap) != 2 {
		t.Fatalf("u0: %#v", arch.Stmts[0])
	}
	if u0.PortMap[0].Formal != "clk" {
		t.Fatalf("u0 portmap formal: %#v", u0.PortMap[0])
	}
	u1 := arch.Stmts[1].(*InstantiationStmt)
	if len(u1.PortMap) != 3 || u1.PortMap[0].Formal != "" { // positional
		t.Fatalf("u1 positional: %#v", u1)
	}
	u2 := arch.Stmts[2].(*InstantiationStmt)
	if u2.Arch != "rtl" {
		t.Fatalf("u2 arch: %#v", u2)
	}
	c0 := arch.Stmts[3].(*InstantiationStmt)
	if c0.UnitKind != 0 || c0.Unit != "comp_x" { // bare component (no keyword)
		t.Fatalf("c0 bare: %#v", c0)
	}
	c1 := arch.Stmts[4].(*InstantiationStmt)
	if c1.UnitKind != COMPONENT {
		t.Fatalf("c1 component: %#v", c1)
	}
	// round-trip
	out := Print(df)
	df2, errs2 := ParseFile(NewFileSet(), "t.vhd", []byte(out))
	if errs2 != nil || !equalAST(df, df2) {
		t.Fatalf("instantiation not AST-stable: errs=%v\n%s", errs2, out)
	}
}

func TestParseGenerate(t *testing.T) {
	src := `architecture rtl of e is
begin
  g0 : for i in 0 to 3 generate
    u : entity work.cell port map (clk => clk);
  end generate g0;
  g1 : if use_fifo generate
    f : entity work.fifo port map (clk => clk);
  end generate;
end architecture;`
	df, errs := ParseFile(NewFileSet(), "t.vhd", []byte(src))
	if errs != nil {
		t.Fatalf("errs: %v", errs)
	}
	arch := df.Units[0].(*ArchitectureBody)
	if len(arch.Stmts) != 2 {
		t.Fatalf("stmts: %d", len(arch.Stmts))
	}
	g0, ok := arch.Stmts[0].(*GenerateStmt)
	if !ok || g0.Kind != FOR || g0.Label != "g0" || g0.Param != "i" || g0.Range == nil || len(g0.Stmts) != 1 {
		t.Fatalf("g0: %#v", arch.Stmts[0])
	}
	if _, ok := g0.Stmts[0].(*InstantiationStmt); !ok {
		t.Fatalf("g0 body: %#v", g0.Stmts[0])
	}
	g1, ok := arch.Stmts[1].(*GenerateStmt)
	if !ok || g1.Kind != IF || g1.Cond == nil || len(g1.Stmts) != 1 {
		t.Fatalf("g1: %#v", arch.Stmts[1])
	}
	// round-trip
	out := Print(df)
	df2, errs2 := ParseFile(NewFileSet(), "t.vhd", []byte(out))
	if errs2 != nil || !equalAST(df, df2) {
		t.Fatalf("generate not AST-stable: errs=%v\n%s", errs2, out)
	}
}

func TestParseProcessSimple(t *testing.T) {
	src := `architecture rtl of e is
begin
  proc : process(clk, rst)
    variable tmp : std_logic;
  begin
    q <= d;
    tmp := a;
    null;
  end process proc;
end architecture;`
	df, errs := ParseFile(NewFileSet(), "t.vhd", []byte(src))
	if errs != nil {
		t.Fatalf("errs: %v", errs)
	}
	arch := df.Units[0].(*ArchitectureBody)
	pr, ok := arch.Stmts[0].(*ProcessStmt)
	if !ok || pr.Label != "proc" || len(pr.Sensitivity) != 2 || len(pr.Decls) != 1 || len(pr.Stmts) != 3 {
		t.Fatalf("process: %#v", arch.Stmts[0])
	}
	if _, ok := pr.Decls[0].(*VariableDecl); !ok {
		t.Fatalf("var decl: %#v", pr.Decls[0])
	}
	if _, ok := pr.Stmts[0].(*SignalAssignStmt); !ok {
		t.Fatalf("sig assign: %#v", pr.Stmts[0])
	}
	if _, ok := pr.Stmts[1].(*VariableAssignStmt); !ok {
		t.Fatalf("var assign: %#v", pr.Stmts[1])
	}
	if _, ok := pr.Stmts[2].(*NullStmt); !ok {
		t.Fatalf("null: %#v", pr.Stmts[2])
	}
	// round-trip
	out := Print(df)
	df2, errs2 := ParseFile(NewFileSet(), "t.vhd", []byte(out))
	if errs2 != nil || !equalAST(df, df2) {
		t.Fatalf("process not AST-stable: errs=%v\n%s", errs2, out)
	}
}

func TestParseIfStmt(t *testing.T) {
	src := `architecture rtl of e is
begin
  process(clk) begin
    if rst = '1' then
      q <= '0';
    elsif en = '1' then
      q <= d;
      n := n + 1;
    else
      q <= q;
    end if;
  end process;
end architecture;`
	df, errs := ParseFile(NewFileSet(), "t.vhd", []byte(src))
	if errs != nil {
		t.Fatalf("errs: %v", errs)
	}
	pr := df.Units[0].(*ArchitectureBody).Stmts[0].(*ProcessStmt)
	ifs, ok := pr.Stmts[0].(*IfStmt)
	if !ok || ifs.Cond == nil || len(ifs.Then) != 1 || len(ifs.Elsifs) != 1 || len(ifs.Else) != 1 {
		t.Fatalf("if: %#v", pr.Stmts[0])
	}
	if len(ifs.Elsifs[0].Stmts) != 2 {
		t.Fatalf("elsif body: %#v", ifs.Elsifs[0])
	}
	// round-trip
	out := Print(df)
	df2, errs2 := ParseFile(NewFileSet(), "t.vhd", []byte(out))
	if errs2 != nil || !equalAST(df, df2) {
		t.Fatalf("if not AST-stable: errs=%v\n%s", errs2, out)
	}
}

func TestParseCaseStmt(t *testing.T) {
	src := `architecture rtl of e is
begin
  process(sel) begin
    case sel is
      when "00" =>
        y <= a;
      when "01" | "10" =>
        y <= b;
      when others =>
        y <= c;
    end case;
  end process;
end architecture;`
	df, errs := ParseFile(NewFileSet(), "t.vhd", []byte(src))
	if errs != nil {
		t.Fatalf("errs: %v", errs)
	}
	pr := df.Units[0].(*ArchitectureBody).Stmts[0].(*ProcessStmt)
	cs, ok := pr.Stmts[0].(*CaseStmt)
	if !ok || cs.Expr == nil || len(cs.Alts) != 3 {
		t.Fatalf("case: %#v", pr.Stmts[0])
	}
	if len(cs.Alts[1].Choices) != 2 {
		t.Fatalf("multi-choice alt: %#v", cs.Alts[1])
	}
	if len(cs.Alts[2].Choices) != 1 { // when others
		t.Fatalf("others alt: %#v", cs.Alts[2])
	}
	// round-trip
	out := Print(df)
	df2, errs2 := ParseFile(NewFileSet(), "t.vhd", []byte(out))
	if errs2 != nil || !equalAST(df, df2) {
		t.Fatalf("case not AST-stable: errs=%v\n%s", errs2, out)
	}
}

func TestParseWaitStmt(t *testing.T) {
	src := `architecture rtl of e is
begin
  process begin
    wait;
    wait for 10 ns;
    wait until clk = '1';
    wait on a, b until c for 5 ns;
  end process;
end architecture;`
	df, errs := ParseFile(NewFileSet(), "t.vhd", []byte(src))
	if errs != nil {
		t.Fatalf("errs: %v", errs)
	}
	st := df.Units[0].(*ArchitectureBody).Stmts[0].(*ProcessStmt).Stmts
	if len(st) != 4 {
		t.Fatalf("stmts: %d", len(st))
	}
	w0 := st[0].(*WaitStmt)
	if len(w0.On) != 0 || w0.Until != nil || w0.For != nil {
		t.Fatalf("wait;: %#v", w0)
	}
	w1 := st[1].(*WaitStmt)
	if w1.For == nil || w1.Until != nil {
		t.Fatalf("wait for: %#v", w1)
	}
	w2 := st[2].(*WaitStmt)
	if w2.Until == nil {
		t.Fatalf("wait until: %#v", w2)
	}
	w3 := st[3].(*WaitStmt)
	if len(w3.On) != 2 || w3.Until == nil || w3.For == nil {
		t.Fatalf("wait on/until/for: %#v", w3)
	}
	// round-trip
	out := Print(df)
	df2, errs2 := ParseFile(NewFileSet(), "t.vhd", []byte(out))
	if errs2 != nil || !equalAST(df, df2) {
		t.Fatalf("wait not AST-stable: errs=%v\n%s", errs2, out)
	}
}

func TestParseForLoop(t *testing.T) {
	src := `architecture rtl of e is
begin
  process(clk) begin
    for i in 0 to 7 loop
      acc := acc + i;
      q <= acc;
    end loop;
  end process;
end architecture;`
	df, errs := ParseFile(NewFileSet(), "t.vhd", []byte(src))
	if errs != nil {
		t.Fatalf("errs: %v", errs)
	}
	pr := df.Units[0].(*ArchitectureBody).Stmts[0].(*ProcessStmt)
	lp, ok := pr.Stmts[0].(*LoopStmt)
	if !ok || lp.Scheme != FOR || lp.Param != "i" || lp.Range == nil || len(lp.Stmts) != 2 {
		t.Fatalf("for-loop: %#v", pr.Stmts[0])
	}
	// round-trip
	out := Print(df)
	df2, errs2 := ParseFile(NewFileSet(), "t.vhd", []byte(out))
	if errs2 != nil || !equalAST(df, df2) {
		t.Fatalf("for-loop not AST-stable: errs=%v\n%s", errs2, out)
	}
}

func TestParseLoopControl(t *testing.T) {
	src := `architecture rtl of e is
begin
  process begin
    while x < 10 loop
      x := x + 1;
      next when x = 5;
      exit;
    end loop;
    loop
      exit when done;
    end loop;
  end process;
end architecture;`
	df, errs := ParseFile(NewFileSet(), "t.vhd", []byte(src))
	if errs != nil {
		t.Fatalf("errs: %v", errs)
	}
	st := df.Units[0].(*ArchitectureBody).Stmts[0].(*ProcessStmt).Stmts
	wl, ok := st[0].(*LoopStmt)
	if !ok || wl.Scheme != WHILE || wl.Cond == nil || len(wl.Stmts) != 3 {
		t.Fatalf("while loop: %#v", st[0])
	}
	if _, ok := wl.Stmts[1].(*NextStmt); !ok {
		t.Fatalf("next: %#v", wl.Stmts[1])
	}
	if e, ok := wl.Stmts[2].(*ExitStmt); !ok || e.When != nil {
		t.Fatalf("exit: %#v", wl.Stmts[2])
	}
	bl, ok := st[1].(*LoopStmt)
	if !ok || bl.Scheme != 0 || bl.Cond != nil {
		t.Fatalf("bare loop: %#v", st[1])
	}
	if e, ok := bl.Stmts[0].(*ExitStmt); !ok || e.When == nil {
		t.Fatalf("exit when: %#v", bl.Stmts[0])
	}
	// round-trip
	out := Print(df)
	df2, errs2 := ParseFile(NewFileSet(), "t.vhd", []byte(out))
	if errs2 != nil || !equalAST(df, df2) {
		t.Fatalf("loop control not AST-stable: errs=%v\n%s", errs2, out)
	}
}

func TestParseReturnStmt(t *testing.T) {
	src := `architecture rtl of e is
begin
  process begin
    return x + 1;
    return;
  end process;
end architecture;`
	df, errs := ParseFile(NewFileSet(), "t.vhd", []byte(src))
	if errs != nil {
		t.Fatalf("errs: %v", errs)
	}
	pr := df.Units[0].(*ArchitectureBody).Stmts[0].(*ProcessStmt)
	r0, ok := pr.Stmts[0].(*ReturnStmt)
	if !ok || r0.Value == nil {
		t.Fatalf("return-with-value: %#v", pr.Stmts[0])
	}
	r1, ok := pr.Stmts[1].(*ReturnStmt)
	if !ok || r1.Value != nil {
		t.Fatalf("bare return: %#v", pr.Stmts[1])
	}
	// round-trip
	out := Print(df)
	df2, errs2 := ParseFile(NewFileSet(), "t.vhd", []byte(out))
	if errs2 != nil || !equalAST(df, df2) {
		t.Fatalf("return not AST-stable: errs=%v\n%s", errs2, out)
	}
}

func TestParseSubprogramBody(t *testing.T) {
	src := `architecture rtl of e is
  function inc(x : integer) return integer is
    variable y : integer;
  begin
    y := x + 1;
    return y;
  end function inc;
  procedure clr(signal s : out std_logic) is
  begin
    s <= '0';
  end procedure;
begin
end architecture;`
	df, errs := ParseFile(NewFileSet(), "t.vhd", []byte(src))
	if errs != nil {
		t.Fatalf("errs: %v", errs)
	}
	arch := df.Units[0].(*ArchitectureBody)
	if len(arch.Decls) != 2 {
		t.Fatalf("decls: %d", len(arch.Decls))
	}
	fb, ok := arch.Decls[0].(*SubprogramBody)
	if !ok || fb.IsProcedure || fb.Designator != "inc" || fb.ReturnMark != "integer" || len(fb.Decls) != 1 || len(fb.Stmts) != 2 {
		t.Fatalf("function body: %#v", arch.Decls[0])
	}
	pb, ok := arch.Decls[1].(*SubprogramBody)
	if !ok || !pb.IsProcedure || pb.Designator != "clr" || len(pb.Stmts) != 1 {
		t.Fatalf("procedure body: %#v", arch.Decls[1])
	}
	// a spec-only declaration (no `is`) still yields a *SubprogramDecl
	dss := parseDecls(t, `function f return integer;`)
	if _, ok := dss[0].(*SubprogramDecl); !ok {
		t.Fatalf("spec-only should be SubprogramDecl: %#v", dss[0])
	}
	// round-trip
	out := Print(df)
	df2, errs2 := ParseFile(NewFileSet(), "t.vhd", []byte(out))
	if errs2 != nil || !equalAST(df, df2) {
		t.Fatalf("subprogram body not AST-stable: errs=%v\n%s", errs2, out)
	}
}

func TestParsePhysicalLiteral(t *testing.T) {
	pl, ok := mustParseExpr(t, "5 ns").(*PhysicalLit)
	if !ok || pl.Value != "5" || pl.Unit != "ns" {
		t.Fatalf("physical literal: %#v", mustParseExpr(t, "5 ns"))
	}
	// a plain numeric (no unit) stays a BasicLit
	if _, ok := mustParseExpr(t, "10").(*BasicLit); !ok {
		t.Fatalf("plain literal should be BasicLit: %#v", mustParseExpr(t, "10"))
	}
	// real-valued physical literal
	if pl, ok := mustParseExpr(t, "1.5 us").(*PhysicalLit); !ok || pl.Unit != "us" {
		t.Fatalf("real physical: %#v", mustParseExpr(t, "1.5 us"))
	}
	// round-trip inside a constant default
	df, errs := ParseFile(NewFileSet(), "t.vhd", []byte("package p is\nconstant period : time := 10 ns;\nend package;"))
	if errs != nil {
		t.Fatalf("errs: %v", errs)
	}
	out := Print(df)
	df2, errs2 := ParseFile(NewFileSet(), "t.vhd", []byte(out))
	if errs2 != nil || !equalAST(df, df2) {
		t.Fatalf("physical literal not AST-stable: errs=%v\n%s", errs2, out)
	}
}

func TestParsePackageBody(t *testing.T) {
	src := `package p is
  function inc(x : integer) return integer;
end package;

package body p is
  constant K : integer := 4;
  function inc(x : integer) return integer is
  begin
    return x + 1;
  end function;
end package body p;`
	df, errs := ParseFile(NewFileSet(), "t.vhd", []byte(src))
	if errs != nil {
		t.Fatalf("errs: %v", errs)
	}
	if len(df.Units) != 2 {
		t.Fatalf("expected 2 units (package + body), got %d", len(df.Units))
	}
	if _, ok := df.Units[0].(*PackageDecl); !ok {
		t.Fatalf("unit0 should be PackageDecl: %#v", df.Units[0])
	}
	pb, ok := df.Units[1].(*PackageBody)
	if !ok || pb.Name != "p" || len(pb.Decls) != 2 {
		t.Fatalf("package body: %#v", df.Units[1])
	}
	if _, ok := pb.Decls[1].(*SubprogramBody); !ok {
		t.Fatalf("body should contain a SubprogramBody: %#v", pb.Decls[1])
	}
	// round-trip
	out := Print(df)
	df2, errs2 := ParseFile(NewFileSet(), "t.vhd", []byte(out))
	if errs2 != nil || !equalAST(df, df2) {
		t.Fatalf("package body not AST-stable: errs=%v\n%s", errs2, out)
	}
}

func TestParseWaveformAfter(t *testing.T) {
	src := `architecture rtl of e is
begin
  clk <= not clk after 5 ns;
  rst <= '1', '0' after 15 ns;
  q <= a;
end architecture;`
	df, errs := ParseFile(NewFileSet(), "t.vhd", []byte(src))
	if errs != nil {
		t.Fatalf("errs: %v", errs)
	}
	arch := df.Units[0].(*ArchitectureBody)
	a0 := arch.Stmts[0].(*ConcurrentSignalAssign)
	if len(a0.Waveform) != 1 || a0.Waveform[0].After == nil {
		t.Fatalf("clk waveform: %#v", a0)
	}
	a1 := arch.Stmts[1].(*ConcurrentSignalAssign)
	if len(a1.Waveform) != 2 || a1.Waveform[0].After != nil || a1.Waveform[1].After == nil {
		t.Fatalf("rst waveform: %#v", a1)
	}
	a2 := arch.Stmts[2].(*ConcurrentSignalAssign)
	if len(a2.Waveform) != 1 || a2.Waveform[0].After != nil {
		t.Fatalf("q waveform: %#v", a2)
	}
	// sequential waveform-after (inside a process)
	src2 := `architecture rtl of e is
begin
  process begin
    d <= x after 2 ns;
  end process;
end architecture;`
	df2, errs2 := ParseFile(NewFileSet(), "t.vhd", []byte(src2))
	if errs2 != nil {
		t.Fatalf("errs2: %v", errs2)
	}
	sa := df2.Units[0].(*ArchitectureBody).Stmts[0].(*ProcessStmt).Stmts[0].(*SignalAssignStmt)
	if len(sa.Waveform) != 1 || sa.Waveform[0].After == nil {
		t.Fatalf("seq waveform: %#v", sa)
	}
	// round-trip both
	for _, s := range []string{src, src2} {
		d, e := ParseFile(NewFileSet(), "t.vhd", []byte(s))
		out := Print(d)
		d2, e2 := ParseFile(NewFileSet(), "t.vhd", []byte(out))
		if e != nil || e2 != nil || !equalAST(d, d2) {
			t.Fatalf("waveform not AST-stable: e=%v e2=%v\n%s", e, e2, out)
		}
	}
}

func TestParseAssertReport(t *testing.T) {
	src := `architecture rtl of e is
begin
  check : assert x = 1 report "bad" severity error;
  process begin
    assert y > 0;
    assert z = 0 report "z nonzero";
    report "done" severity note;
  end process;
end architecture;`
	df, errs := ParseFile(NewFileSet(), "t.vhd", []byte(src))
	if errs != nil {
		t.Fatalf("errs: %v", errs)
	}
	arch := df.Units[0].(*ArchitectureBody)
	ca, ok := arch.Stmts[0].(*AssertStmt)
	if !ok || ca.Label != "check" || ca.Cond == nil || ca.Report == nil || ca.Severity == nil {
		t.Fatalf("concurrent assert: %#v", arch.Stmts[0])
	}
	pst := arch.Stmts[1].(*ProcessStmt).Stmts
	a0 := pst[0].(*AssertStmt)
	if a0.Cond == nil || a0.Report != nil || a0.Severity != nil {
		t.Fatalf("seq assert bare: %#v", a0)
	}
	a1 := pst[1].(*AssertStmt)
	if a1.Report == nil || a1.Severity != nil {
		t.Fatalf("seq assert+report: %#v", a1)
	}
	r := pst[2].(*ReportStmt)
	if r.Report == nil || r.Severity == nil {
		t.Fatalf("report: %#v", r)
	}
	// round-trip
	out := Print(df)
	df2, errs2 := ParseFile(NewFileSet(), "t.vhd", []byte(out))
	if errs2 != nil || !equalAST(df, df2) {
		t.Fatalf("assert/report not AST-stable: errs=%v\n%s", errs2, out)
	}
}

func TestParseProcedureCall(t *testing.T) {
	src := `architecture rtl of e is
begin
  mon : monitor(clk, rst);          -- concurrent procedure call (labeled, positional)
  process begin
    test_equal(a, b);                -- sequential procedure call (positional)
    done;                            -- sequential parameterless call
  end process;
end architecture;`
	df, errs := ParseFile(NewFileSet(), "t.vhd", []byte(src))
	if errs != nil {
		t.Fatalf("errs: %v", errs)
	}
	arch := df.Units[0].(*ArchitectureBody)
	cc, ok := arch.Stmts[0].(*ProcedureCallStmt)
	if !ok || cc.Label != "mon" || cc.Name != "monitor" || len(cc.Args) != 2 {
		t.Fatalf("concurrent proc call: %#v", arch.Stmts[0])
	}
	pst := arch.Stmts[1].(*ProcessStmt).Stmts
	s0, ok := pst[0].(*ProcedureCallStmt)
	if !ok || s0.Name != "test_equal" || len(s0.Args) != 2 {
		t.Fatalf("seq proc call: %#v", pst[0])
	}
	s1, ok := pst[1].(*ProcedureCallStmt)
	if !ok || s1.Name != "done" || len(s1.Args) != 0 {
		t.Fatalf("parameterless call: %#v", pst[1])
	}
	// round-trip
	out := Print(df)
	df2, errs2 := ParseFile(NewFileSet(), "t.vhd", []byte(out))
	if errs2 != nil || !equalAST(df, df2) {
		t.Fatalf("procedure call not AST-stable: errs=%v\n%s", errs2, out)
	}
}

func TestParseConfigurationBlock(t *testing.T) {
	// block-only configuration (no component configs): nested blocks + use clause.
	src := `configuration cfg of e is
  use work.pkg.all;
  for rtl
    for gen_block
    end for;
  end for;
end configuration cfg;`
	df, errs := ParseFile(NewFileSet(), "t.vhd", []byte(src))
	if errs != nil {
		t.Fatalf("errs: %v", errs)
	}
	cfg, ok := df.Units[0].(*ConfigurationDecl)
	if !ok || cfg.Name != "cfg" || cfg.Entity != "e" || len(cfg.Decls) != 1 || cfg.Block == nil {
		t.Fatalf("config: %#v", df.Units[0])
	}
	if cfg.Block.Spec != "rtl" || len(cfg.Block.Items) != 1 {
		t.Fatalf("top block: %#v", cfg.Block)
	}
	if nb, ok := cfg.Block.Items[0].(*BlockConfig); !ok || nb.Spec != "gen_block" {
		t.Fatalf("nested block: %#v", cfg.Block.Items[0])
	}
	// round-trip
	out := Print(df)
	df2, errs2 := ParseFile(NewFileSet(), "t.vhd", []byte(out))
	if errs2 != nil || !equalAST(df, df2) {
		t.Fatalf("config not AST-stable: errs=%v\n%s", errs2, out)
	}
}

func TestParseSharedVariable(t *testing.T) {
	ds := parseDecls(t, `
		shared variable s : integer := 0;
		variable v : bit;`)
	if len(ds) != 2 {
		t.Fatalf("got %d decls", len(ds))
	}
	sv, ok := ds[0].(*VariableDecl)
	if !ok || !sv.Shared || sv.Names[0] != "s" || sv.Default == nil {
		t.Fatalf("shared variable: %#v", ds[0])
	}
	pv, ok := ds[1].(*VariableDecl)
	if !ok || pv.Shared {
		t.Fatalf("plain variable should not be Shared: %#v", ds[1])
	}
	// round-trip
	df, errs := ParseFile(NewFileSet(), "t.vhd", []byte("package p is\nshared variable s : integer := 0;\nend package;"))
	if errs != nil {
		t.Fatalf("errs: %v", errs)
	}
	out := Print(df)
	df2, errs2 := ParseFile(NewFileSet(), "t.vhd", []byte(out))
	if errs2 != nil || !equalAST(df, df2) {
		t.Fatalf("shared variable not AST-stable: errs=%v\n%s", errs2, out)
	}
}

func TestParseComponentConfiguration(t *testing.T) {
	src := `configuration cfg of e is
  for rtl
    for all : ram
      use configuration work.ram_sim;
    end for;
    for u0 : comp
      use entity work.ent(rtl) port map (clk => clk, q => open);
    end for;
    for others : cache
      use entity work.cache_impl;
    end for;
  end for;
end configuration;`
	df, errs := ParseFile(NewFileSet(), "t.vhd", []byte(src))
	if errs != nil {
		t.Fatalf("errs: %v", errs)
	}
	top := df.Units[0].(*ConfigurationDecl).Block
	if len(top.Items) != 3 {
		t.Fatalf("items: %d", len(top.Items))
	}
	c0, ok := top.Items[0].(*ComponentConfig)
	if !ok || len(c0.Insts) != 1 || c0.Insts[0] != "all" || c0.Comp != "ram" || c0.Binding == nil || c0.Binding.UnitKind != CONFIGURATION || c0.Binding.Unit != "work.ram_sim" {
		t.Fatalf("c0: %#v", top.Items[0])
	}
	c1, ok := top.Items[1].(*ComponentConfig)
	if !ok || c1.Insts[0] != "u0" || c1.Binding.UnitKind != ENTITY || c1.Binding.Arch != "rtl" || len(c1.Binding.PortMap) != 2 {
		t.Fatalf("c1: %#v", top.Items[1])
	}
	c2 := top.Items[2].(*ComponentConfig)
	if c2.Insts[0] != "others" || c2.Binding.UnitKind != ENTITY || c2.Binding.Arch != "" {
		t.Fatalf("c2: %#v", top.Items[2])
	}
	// round-trip
	out := Print(df)
	df2, errs2 := ParseFile(NewFileSet(), "t.vhd", []byte(out))
	if errs2 != nil || !equalAST(df, df2) {
		t.Fatalf("component config not AST-stable: errs=%v\n%s", errs2, out)
	}
}

func TestParseConfigurationSpec(t *testing.T) {
	src := `architecture rtl of e is
  for u0 : comp use entity work.ent(rtl) port map (clk => clk);
  for all : ram use configuration work.ram_sim;
begin
end architecture;`
	df, errs := ParseFile(NewFileSet(), "t.vhd", []byte(src))
	if errs != nil {
		t.Fatalf("errs: %v", errs)
	}
	arch := df.Units[0].(*ArchitectureBody)
	if len(arch.Decls) != 2 {
		t.Fatalf("decls: %d", len(arch.Decls))
	}
	cs0, ok := arch.Decls[0].(*ConfigSpec)
	if !ok || cs0.Insts[0] != "u0" || cs0.Comp != "comp" || cs0.Binding == nil || cs0.Binding.UnitKind != ENTITY || cs0.Binding.Arch != "rtl" || len(cs0.Binding.PortMap) != 1 {
		t.Fatalf("config spec 0: %#v", arch.Decls[0])
	}
	cs1, ok := arch.Decls[1].(*ConfigSpec)
	if !ok || cs1.Insts[0] != "all" || cs1.Binding.UnitKind != CONFIGURATION {
		t.Fatalf("config spec 1: %#v", arch.Decls[1])
	}
	// round-trip
	out := Print(df)
	df2, errs2 := ParseFile(NewFileSet(), "t.vhd", []byte(out))
	if errs2 != nil || !equalAST(df, df2) {
		t.Fatalf("config spec not AST-stable: errs=%v\n%s", errs2, out)
	}
}

func TestParseFileAndAccessAndUse(t *testing.T) {
	ds := parseDecls(t, `
		use work.textio.all;
		type ft is file of character;
		type ap is access rec_t;
		file f1 : text;
		file f2 : text open read_mode is "data.txt";`)
	if len(ds) != 5 {
		t.Fatalf("got %d decls", len(ds))
	}
	if _, ok := ds[0].(*UseClause); !ok {
		t.Fatalf("use clause as decl: %#v", ds[0])
	}
	td0 := ds[1].(*TypeDecl)
	if ftd, ok := td0.Def.(*FileTypeDef); !ok || ftd.Mark != "character" {
		t.Fatalf("file type def: %#v", td0.Def)
	}
	td1 := ds[2].(*TypeDecl)
	if ad, ok := td1.Def.(*AccessDef); !ok || ad.Mark != "rec_t" {
		t.Fatalf("access type def: %#v", td1.Def)
	}
	f1 := ds[3].(*FileDecl)
	if f1.Names[0] != "f1" || f1.SubtypeMark != "text" || f1.LogicalName != nil {
		t.Fatalf("file decl 1: %#v", f1)
	}
	f2 := ds[4].(*FileDecl)
	if f2.OpenMode == nil || f2.LogicalName == nil {
		t.Fatalf("file decl 2 (open..is): %#v", f2)
	}
	// round-trip
	df, errs := ParseFile(NewFileSet(), "t.vhd", []byte("package p is\nuse work.textio.all;\nfile f2 : text open read_mode is \"data.txt\";\ntype ft is file of character;\nend package;"))
	if errs != nil {
		t.Fatalf("errs: %v", errs)
	}
	out := Print(df)
	df2, errs2 := ParseFile(NewFileSet(), "t.vhd", []byte(out))
	if errs2 != nil || !equalAST(df, df2) {
		t.Fatalf("file/access/use not AST-stable: errs=%v\n%s", errs2, out)
	}
}

func TestParseFileDeclModes(t *testing.T) {
	// VHDL-87 form: file f : text is out "cpu0.acc";
	ds := parseDecls(t, `file f0 : text is out "cpu0.acc";`)
	if len(ds) != 1 {
		t.Fatalf("expected 1 decl, got %d", len(ds))
	}
	f0 := ds[0].(*FileDecl)
	if f0.Names[0] != "f0" || f0.SubtypeMark != "text" || f0.Mode != "out" || f0.LogicalName == nil || f0.OpenMode != nil {
		t.Fatalf("file decl out mode: %#v", f0)
	}
	// VHDL-87 form: file f1 : text is in "input.txt";
	ds2 := parseDecls(t, `file f1 : text is in "input.txt";`)
	if len(ds2) != 1 {
		t.Fatalf("expected 1 decl, got %d", len(ds2))
	}
	f1 := ds2[0].(*FileDecl)
	if f1.Names[0] != "f1" || f1.SubtypeMark != "text" || f1.Mode != "in" || f1.LogicalName == nil || f1.OpenMode != nil {
		t.Fatalf("file decl in mode: %#v", f1)
	}
	// regression: no-is form (Mode must be "")
	ds3 := parseDecls(t, `file f : text;`)
	f3 := ds3[0].(*FileDecl)
	if f3.Mode != "" || f3.LogicalName != nil {
		t.Fatalf("file decl no-is: %#v", f3)
	}
	// regression: open form (Mode must be "")
	ds4 := parseDecls(t, `file fp : text open write_mode is "n";`)
	f4 := ds4[0].(*FileDecl)
	if f4.Mode != "" || f4.OpenMode == nil || f4.LogicalName == nil {
		t.Fatalf("file decl open form: %#v", f4)
	}
	// round-trip: VHDL-87 out mode
	dfOut, errs := ParseFile(NewFileSet(), "t.vhd", []byte("package p is\nfile f0 : text is out \"cpu0.acc\";\nend package;"))
	if errs != nil {
		t.Fatalf("parse errs: %v", errs)
	}
	outStr := Print(dfOut)
	dfOut2, errs2 := ParseFile(NewFileSet(), "t.vhd", []byte(outStr))
	if errs2 != nil || !equalAST(dfOut, dfOut2) {
		t.Fatalf("file decl out mode not AST-stable: errs=%v\n%s", errs2, outStr)
	}
	// round-trip: VHDL-87 in mode
	dfIn, errs := ParseFile(NewFileSet(), "t.vhd", []byte("package p is\nfile f1 : text is in \"input.txt\";\nend package;"))
	if errs != nil {
		t.Fatalf("parse errs: %v", errs)
	}
	outStr = Print(dfIn)
	dfIn2, errs2 := ParseFile(NewFileSet(), "t.vhd", []byte(outStr))
	if errs2 != nil || !equalAST(dfIn, dfIn2) {
		t.Fatalf("file decl in mode not AST-stable: errs=%v\n%s", errs2, outStr)
	}
}

func TestParseNameSuffixChains(t *testing.T) {
	// double index: arr(i)(j) -> CallExpr{ Fun: CallExpr{arr,[i]}, [j] }
	e := mustParseExpr(t, "arr(i)(j)")
	outer, ok := e.(*CallExpr)
	if !ok {
		t.Fatalf("double index top: %#v", e)
	}
	if _, ok := outer.Fun.(*CallExpr); !ok {
		t.Fatalf("double index inner: %#v", outer.Fun)
	}
	// selection after index: arr(i).field -> SelectorExpr{ X: CallExpr{arr,[i]}, Sel: "field" }
	se, ok := mustParseExpr(t, "arr(i).field").(*SelectorExpr)
	if !ok || se.Sel != "field" {
		t.Fatalf("post-index selection: %#v", mustParseExpr(t, "arr(i).field"))
	}
	if _, ok := se.X.(*CallExpr); !ok {
		t.Fatalf("selector base: %#v", se.X)
	}
	// SelectorExpr.End() must point just past the selector (dot pos + ".field" len).
	se2 := mustParseExpr(t, "arr(i).field").(*SelectorExpr)
	if int(se2.End()-se2.Dot) != len(".field") {
		t.Fatalf("SelectorExpr End/Dot span: End-Dot=%d, want %d", se2.End()-se2.Dot, len(".field"))
	}
	// invariants: flat dotted name stays a flat Ident; single call stays one CallExpr
	if id, ok := mustParseExpr(t, "a.b.c").(*Ident); !ok || id.Name != "a.b.c" {
		t.Fatalf("flat dotted name regressed: %#v", mustParseExpr(t, "a.b.c"))
	}
	if c, ok := mustParseExpr(t, "f(x)").(*CallExpr); !ok || len(c.Args) != 1 {
		t.Fatalf("single call regressed: %#v", mustParseExpr(t, "f(x)"))
	}
	// round-trip a mixed suffix name as a signal-assignment target
	src := `architecture rtl of e is
begin
  rec.d(STABLE)(19 downto 4) <= x;
end architecture;`
	df, errs := ParseFile(NewFileSet(), "t.vhd", []byte(src))
	if errs != nil {
		t.Fatalf("errs: %v", errs)
	}
	out := Print(df)
	df2, errs2 := ParseFile(NewFileSet(), "t.vhd", []byte(out))
	if errs2 != nil || !equalAST(df, df2) {
		t.Fatalf("suffix-chain name not AST-stable: errs=%v\n%s", errs2, out)
	}
}

func TestParseNamedCallArgs(t *testing.T) {
	// named association
	e := mustParseExpr(t, "f(clk => c, rst => r)")
	ce, ok := e.(*CallExpr)
	if !ok || len(ce.Args) != 2 || ce.Args[0].Formal != "clk" || ce.Args[1].Formal != "rst" {
		t.Fatalf("named call: %#v", e)
	}
	// positional (Formal == "")
	cp, ok := mustParseExpr(t, "f(a, b)").(*CallExpr)
	if !ok || len(cp.Args) != 2 || cp.Args[0].Formal != "" {
		t.Fatalf("positional call: %#v", cp)
	}
	// indexed name (still a CallExpr, positional)
	ci, ok := mustParseExpr(t, "arr(i)").(*CallExpr)
	if !ok || len(ci.Args) != 1 || ci.Args[0].Formal != "" {
		t.Fatalf("indexed: %#v", ci)
	}
	// round-trip both forms (as constant defaults)
	for _, src := range []string{"f(clk => c, rst => r)", "f(a, b)"} {
		df, errs := ParseFile(NewFileSet(), "t.vhd", []byte("package p is\nconstant k : t := "+src+";\nend package;"))
		out := Print(df)
		df2, errs2 := ParseFile(NewFileSet(), "t.vhd", []byte(out))
		if errs != nil || errs2 != nil || !equalAST(df, df2) {
			t.Fatalf("call args not AST-stable for %q: e=%v e2=%v\n%s", src, errs, errs2, out)
		}
	}
}

func TestParseSelectedAssign(t *testing.T) {
	src := `architecture rtl of e is
begin
  with sel select
    q <= a when "00",
         b when "01" | "10",
         c when others;
end architecture;`
	df, errs := ParseFile(NewFileSet(), "t.vhd", []byte(src))
	if errs != nil {
		t.Fatalf("errs: %v", errs)
	}
	sa, ok := df.Units[0].(*ArchitectureBody).Stmts[0].(*SelectedSignalAssign)
	if !ok || sa.Expr == nil || sa.Target == nil || len(sa.Alts) != 3 {
		t.Fatalf("selected assign: %#v", df.Units[0].(*ArchitectureBody).Stmts[0])
	}
	if len(sa.Alts[1].Choices) != 2 { // "01" | "10"
		t.Fatalf("multi-choice alt: %#v", sa.Alts[1])
	}
	if len(sa.Alts[0].Waveform) != 1 {
		t.Fatalf("alt0 waveform: %#v", sa.Alts[0])
	}
	// round-trip
	out := Print(df)
	df2, errs2 := ParseFile(NewFileSet(), "t.vhd", []byte(out))
	if errs2 != nil || !equalAST(df, df2) {
		t.Fatalf("selected assign not AST-stable: errs=%v\n%s", errs2, out)
	}
}

func TestParseForRangeTypeMark(t *testing.T) {
	// for loop: `for i in integer range 0 to 7 loop`
	src := `architecture a of e is
begin
  process begin
    for i in integer range 0 to 7 loop
      null;
    end loop;
  end process;
end architecture;`
	df, errs := ParseFile(NewFileSet(), "t.vhd", []byte(src))
	if errs != nil {
		t.Fatalf("errs: %v", errs)
	}
	pr := df.Units[0].(*ArchitectureBody).Stmts[0].(*ProcessStmt)
	lp, ok := pr.Stmts[0].(*LoopStmt)
	if !ok || lp.Scheme != FOR || lp.Param != "i" {
		t.Fatalf("for-loop: %#v", pr.Stmts[0])
	}
	rc, ok := lp.Range.(*RangeConstraint)
	if !ok {
		t.Fatalf("loop range is %T, want *RangeConstraint", lp.Range)
	}
	id, ok := rc.Mark.(*Ident)
	if !ok || id.Name != "integer" {
		t.Fatalf("RangeConstraint.Mark: %#v", rc.Mark)
	}
	if _, ok := rc.Range.(*Range); !ok {
		t.Fatalf("RangeConstraint.Range: %T", rc.Range)
	}
	// round-trip
	out := Print(df)
	df2, errs2 := ParseFile(NewFileSet(), "t.vhd", []byte(out))
	if errs2 != nil || !equalAST(df, df2) {
		t.Fatalf("for-range type mark not AST-stable: errs=%v\n%s", errs2, out)
	}

	// for generate: `for g in 0 to 3 generate` — plain Range, NOT RangeConstraint
	src2 := `architecture a of e is
begin
  g : for g in 0 to 3 generate
    u : entity work.cell port map (clk => clk);
  end generate;
end architecture;`
	df3, errs3 := ParseFile(NewFileSet(), "t.vhd", []byte(src2))
	if errs3 != nil {
		t.Fatalf("generate errs: %v", errs3)
	}
	gs, ok := df3.Units[0].(*ArchitectureBody).Stmts[0].(*GenerateStmt)
	if !ok || gs.Kind != FOR || gs.Param != "g" {
		t.Fatalf("generate: %#v", df3.Units[0].(*ArchitectureBody).Stmts[0])
	}
	if _, ok := gs.Range.(*Range); !ok {
		t.Fatalf("generate plain range is %T, want *Range", gs.Range)
	}
	// round-trip
	out3 := Print(df3)
	df4, errs4 := ParseFile(NewFileSet(), "t.vhd", []byte(out3))
	if errs4 != nil || !equalAST(df3, df4) {
		t.Fatalf("generate plain range not AST-stable: errs=%v\n%s", errs4, out3)
	}
}

func TestParseAllocator(t *testing.T) {
	// Case 1: new integer → AllocatorExpr with X = *Ident "integer"
	e1 := mustParseExpr(t, "new integer")
	a1, ok := e1.(*AllocatorExpr)
	if !ok {
		t.Fatalf("new integer: expected *AllocatorExpr, got %T", e1)
	}
	id, ok := a1.X.(*Ident)
	if !ok || id.Name != "integer" {
		t.Fatalf("new integer: X = %#v, want *Ident{\"integer\"}", a1.X)
	}

	// Case 2: new bit_vector(7 downto 0) → AllocatorExpr with X = *CallExpr
	e2 := mustParseExpr(t, "new bit_vector(7 downto 0)")
	a2, ok := e2.(*AllocatorExpr)
	if !ok {
		t.Fatalf("new bit_vector(...): expected *AllocatorExpr, got %T", e2)
	}
	if _, ok := a2.X.(*CallExpr); !ok {
		t.Fatalf("new bit_vector(...): X = %T, want *CallExpr", a2.X)
	}

	// Case 3: new string'("x") → AllocatorExpr (qualified expression)
	e3 := mustParseExpr(t, `new string'("x")`)
	a3, ok := e3.(*AllocatorExpr)
	if !ok {
		t.Fatalf(`new string'("x"): expected *AllocatorExpr, got %T`, e3)
	}
	if _, ok := a3.X.(*QualifiedExpr); !ok {
		t.Fatalf(`new string'("x"): X = %T, want *QualifiedExpr`, a3.X)
	}

	// Round-trip all three via expression print/reparse
	for _, src := range []string{
		"new integer",
		"new bit_vector(7 downto 0)",
		`new string'("x")`,
	} {
		e := mustParseExpr(t, src)
		var b strings.Builder
		printExpr(&b, e)
		got := b.String()
		e2 := mustParseExpr(t, got)
		if !equalAST(e, e2) {
			t.Fatalf("allocator %q: round-trip mismatch: printed %q", src, got)
		}
	}
}

func TestParseWithCPP(t *testing.T) {
	// bad-exe: must NOT panic, must return errors; no gcc guard — works without gcc.
	t.Run("bad_exe", func(t *testing.T) {
		_, err := ParseFile(NewFileSet(), "t.vhd", []byte("entity e is end entity;\n"), WithCPP("definitely-not-a-real-cpp-xyz"))
		var ce *CPPError
		if !errors.As(err, &ce) {
			t.Fatalf("expected *CPPError for bad cpp exe, got %v", err)
		}
	})

	// option-is-the-enabler: WITHOUT WithCPP, '#' is illegal, so we get errors.
	t.Run("no_cpp_hash_fails", func(t *testing.T) {
		_, errs := ParseFile(NewFileSet(), "t.vhd", []byte("#define FOO\nentity e is end entity;\n"))
		if errs == nil {
			t.Fatal("expected errors: '#' is illegal in VHDL without cpp")
		}
	})

	// gcc-dependent tests.
	if _, err := exec.LookPath("gcc"); err != nil {
		t.Skip("gcc not available")
	}

	// macro/define consumed: #define FOO → entity parses cleanly and round-trips.
	t.Run("define_consumed", func(t *testing.T) {
		src := "#define FOO\nentity e is end entity;\n"
		f1, errs := ParseFile(NewFileSet(), "t.vhd", []byte(src), WithCPP("gcc"))
		if errs != nil {
			t.Fatalf("unexpected errors: %v", errs)
		}
		out := Print(f1)
		f2, errs2 := ParseFile(NewFileSet(), "t.vhd", []byte(out))
		if errs2 != nil {
			t.Fatalf("reparse errors: %v", errs2)
		}
		if !equalAST(f1, f2) {
			t.Fatal("AST not stable across print/reparse")
		}
	})

	// token-paste: MK(foo) expands to foo_t; entity name must be foo_t.
	t.Run("token_paste", func(t *testing.T) {
		src := "#define MK(x) x ## _t\nentity MK(foo) is end entity;\n"
		f1, errs := ParseFile(NewFileSet(), "t.vhd", []byte(src), WithCPP("gcc"))
		if errs != nil {
			t.Fatalf("unexpected errors: %v", errs)
		}
		// Assert entity name is foo_t.
		if len(f1.Units) == 0 {
			t.Fatal("no units")
		}
		ent, ok := f1.Units[0].(*EntityDecl)
		if !ok {
			t.Fatalf("expected EntityDecl, got %T", f1.Units[0])
		}
		if ent.Name != "foo_t" {
			t.Fatalf("expected entity name foo_t, got %q", ent.Name)
		}
		// Also round-trip.
		out := Print(f1)
		f2, errs2 := ParseFile(NewFileSet(), "t.vhd", []byte(out))
		if errs2 != nil {
			t.Fatalf("reparse errors: %v", errs2)
		}
		if !equalAST(f1, f2) {
			t.Fatal("AST not stable across print/reparse")
		}
	})
}

// roundTripSrc is a helper that parses src, asserts zero errors, prints, reparses,
// and checks AST equality.  It returns the first design unit as a convenience.
func roundTripSrc(t *testing.T, src string) DesignUnit {
	t.Helper()
	df, errs := ParseFile(NewFileSet(), "t.vhd", []byte(src))
	if errs != nil {
		t.Fatalf("parse errors: %v", errs)
	}
	out := Print(df)
	df2, errs2 := ParseFile(NewFileSet(), "t.vhd", []byte(out))
	if errs2 != nil {
		t.Fatalf("reparse errors: %v\n--- printed ---\n%s", errs2, out)
	}
	if !equalAST(df, df2) {
		t.Fatalf("AST not stable across print/reparse\n--- printed ---\n%s", out)
	}
	if len(df.Units) == 0 {
		t.Fatal("no design units")
	}
	return df.Units[0]
}

func TestParseEntityStatementPart(t *testing.T) {
	// Case 1: bare assert in entity statement part.
	src1 := "entity e is\nbegin\n  assert true report \"x\" severity note;\nend entity;\n"
	u1 := roundTripSrc(t, src1)
	e1, ok := u1.(*EntityDecl)
	if !ok {
		t.Fatalf("expected *EntityDecl, got %T", u1)
	}
	if len(e1.Stmts) != 1 {
		t.Fatalf("case1: want 1 stmt, got %d", len(e1.Stmts))
	}

	// Case 2: ports + statement part with a labelled assert.
	src2 := "entity e is\n  port(a : in bit);\nbegin\n  check: assert a = '1';\nend entity e;\n"
	u2 := roundTripSrc(t, src2)
	e2, ok := u2.(*EntityDecl)
	if !ok {
		t.Fatalf("expected *EntityDecl, got %T", u2)
	}
	if len(e2.Stmts) != 1 {
		t.Fatalf("case2: want 1 stmt, got %d", len(e2.Stmts))
	}

	// Case 3: regression — entity without statement part still parses cleanly.
	src3 := "entity e is\nend entity;\n"
	u3 := roundTripSrc(t, src3)
	e3, ok := u3.(*EntityDecl)
	if !ok {
		t.Fatalf("expected *EntityDecl, got %T", u3)
	}
	if len(e3.Stmts) != 0 {
		t.Fatalf("case3: want 0 stmts, got %d", len(e3.Stmts))
	}
}

// TestParseMultiUnitContext verifies that library/use context clauses are
// accepted at any top-level position (leading OR between design units).
func TestParseMultiUnitContext(t *testing.T) {
	// Sub-case 1: context clause between two design units.
	src1 := "library a;\nentity e is end entity;\nlibrary b;\nuse b.x.all;\narchitecture r of e is begin end architecture;\n"
	df1, errs1 := ParseFile(NewFileSet(), "t.vhd", []byte(src1))
	if errs1 != nil {
		t.Fatalf("case1: parse errors: %v", errs1)
	}
	if len(df1.Context) != 3 {
		t.Fatalf("case1: want 3 context items, got %d", len(df1.Context))
	}
	if len(df1.Units) != 2 {
		t.Fatalf("case1: want 2 units, got %d", len(df1.Units))
	}
	// round-trip: printer hoists context to top; reparse must yield equalAST
	out1 := Print(df1)
	df1b, errs1b := ParseFile(NewFileSet(), "t.vhd", []byte(out1))
	if errs1b != nil {
		t.Fatalf("case1: reparse errors: %v\n--- printed ---\n%s", errs1b, out1)
	}
	if !equalAST(df1, df1b) {
		t.Fatalf("case1: AST not stable across print/reparse\n--- printed ---\n%s", out1)
	}

	// Sub-case 2: secondary use-led unit (use clause after first entity).
	src2 := "entity e is end entity;\nuse work.p.all;\narchitecture r of e is begin end architecture;\n"
	df2, errs2 := ParseFile(NewFileSet(), "t.vhd", []byte(src2))
	if errs2 != nil {
		t.Fatalf("case2: parse errors: %v", errs2)
	}
	if len(df2.Context) != 1 {
		t.Fatalf("case2: want 1 context item, got %d", len(df2.Context))
	}
	if len(df2.Units) != 2 {
		t.Fatalf("case2: want 2 units, got %d", len(df2.Units))
	}
	out2 := Print(df2)
	df2b, errs2b := ParseFile(NewFileSet(), "t.vhd", []byte(out2))
	if errs2b != nil {
		t.Fatalf("case2: reparse errors: %v\n--- printed ---\n%s", errs2b, out2)
	}
	if !equalAST(df2, df2b) {
		t.Fatalf("case2: AST not stable across print/reparse\n--- printed ---\n%s", out2)
	}

	// Sub-case 3: regression — leading context clauses still work (no regression).
	src3 := "library ieee;\nuse ieee.std_logic_1164.all;\nentity e is end entity;\n"
	df3, errs3 := ParseFile(NewFileSet(), "t.vhd", []byte(src3))
	if errs3 != nil {
		t.Fatalf("case3: parse errors: %v", errs3)
	}
	if len(df3.Context) != 2 {
		t.Fatalf("case3: want 2 context items, got %d", len(df3.Context))
	}
	if len(df3.Units) != 1 {
		t.Fatalf("case3: want 1 unit, got %d", len(df3.Units))
	}
	out3 := Print(df3)
	df3b, errs3b := ParseFile(NewFileSet(), "t.vhd", []byte(out3))
	if errs3b != nil {
		t.Fatalf("case3: reparse errors: %v\n--- printed ---\n%s", errs3b, out3)
	}
	if !equalAST(df3, df3b) {
		t.Fatalf("case3: AST not stable across print/reparse\n--- printed ---\n%s", out3)
	}
}

// TestParseDelayMechanism verifies parsing and round-trip of the optional signal-
// assignment delay mechanism: transport, inertial, and reject <time> inertial.
func TestParseDelayMechanism(t *testing.T) {
	// Helper: wrap a concurrent signal assignment in a minimal architecture.
	concurrentArch := func(assign string) string {
		return "architecture rtl of e is\nbegin\n  " + assign + "\nend architecture;\n"
	}
	// Helper: wrap a sequential signal assignment in a process inside an architecture.
	sequentialProc := func(assign string) string {
		return "architecture rtl of e is\nbegin\n  process\n  begin\n    " + assign + "\n  end process;\nend architecture;\n"
	}

	// Case 1: concurrent transport delay — q <= transport d after 5 ns;
	t.Run("concurrent_transport", func(t *testing.T) {
		src := concurrentArch("q <= transport d after 5 ns;")
		u := roundTripSrc(t, src)
		arch, ok := u.(*ArchitectureBody)
		if !ok {
			t.Fatalf("expected *ArchitectureBody, got %T", u)
		}
		sa, ok := arch.Stmts[0].(*ConcurrentSignalAssign)
		if !ok {
			t.Fatalf("expected *ConcurrentSignalAssign, got %T", arch.Stmts[0])
		}
		if sa.Delay == nil {
			t.Fatal("Delay should not be nil")
		}
		if !sa.Delay.Transport {
			t.Fatalf("expected Transport=true, got false")
		}
	})

	// Case 2: sequential transport delay — q <= transport d;
	t.Run("sequential_transport", func(t *testing.T) {
		src := sequentialProc("q <= transport d;")
		u := roundTripSrc(t, src)
		arch, ok := u.(*ArchitectureBody)
		if !ok {
			t.Fatalf("expected *ArchitectureBody, got %T", u)
		}
		proc, ok := arch.Stmts[0].(*ProcessStmt)
		if !ok {
			t.Fatalf("expected *ProcessStmt, got %T", arch.Stmts[0])
		}
		sa, ok := proc.Stmts[0].(*SignalAssignStmt)
		if !ok {
			t.Fatalf("expected *SignalAssignStmt, got %T", proc.Stmts[0])
		}
		if sa.Delay == nil {
			t.Fatal("Delay should not be nil")
		}
		if !sa.Delay.Transport {
			t.Fatalf("expected Transport=true, got false")
		}
	})

	// Case 3: reject <time> inertial — q <= reject 2 ns inertial d;
	t.Run("reject_inertial", func(t *testing.T) {
		src := concurrentArch("q <= reject 2 ns inertial d;")
		u := roundTripSrc(t, src)
		arch, ok := u.(*ArchitectureBody)
		if !ok {
			t.Fatalf("expected *ArchitectureBody, got %T", u)
		}
		sa, ok := arch.Stmts[0].(*ConcurrentSignalAssign)
		if !ok {
			t.Fatalf("expected *ConcurrentSignalAssign, got %T", arch.Stmts[0])
		}
		if sa.Delay == nil {
			t.Fatal("Delay should not be nil")
		}
		if sa.Delay.Transport {
			t.Fatal("expected Transport=false for reject inertial")
		}
		if sa.Delay.Reject == nil {
			t.Fatal("expected Reject != nil for reject inertial")
		}
	})

	// Case 4: plain inertial — q <= inertial d;
	t.Run("plain_inertial", func(t *testing.T) {
		src := concurrentArch("q <= inertial d;")
		u := roundTripSrc(t, src)
		arch, ok := u.(*ArchitectureBody)
		if !ok {
			t.Fatalf("expected *ArchitectureBody, got %T", u)
		}
		sa, ok := arch.Stmts[0].(*ConcurrentSignalAssign)
		if !ok {
			t.Fatalf("expected *ConcurrentSignalAssign, got %T", arch.Stmts[0])
		}
		if sa.Delay == nil {
			t.Fatal("Delay should not be nil")
		}
		if sa.Delay.Transport {
			t.Fatal("expected Transport=false for inertial")
		}
		if sa.Delay.Reject != nil {
			t.Fatal("expected Reject == nil for plain inertial")
		}
	})

	// Case 5: regression — q <= d; should have Delay == nil (unchanged behavior)
	t.Run("no_delay_regression", func(t *testing.T) {
		src := concurrentArch("q <= d;")
		u := roundTripSrc(t, src)
		arch, ok := u.(*ArchitectureBody)
		if !ok {
			t.Fatalf("expected *ArchitectureBody, got %T", u)
		}
		sa, ok := arch.Stmts[0].(*ConcurrentSignalAssign)
		if !ok {
			t.Fatalf("expected *ConcurrentSignalAssign, got %T", arch.Stmts[0])
		}
		if sa.Delay != nil {
			t.Fatalf("expected Delay == nil for plain assignment, got %+v", sa.Delay)
		}
	})
}

func TestParseSuffixAttribute(t *testing.T) {
	// Case 1: arr(i)'length — AttributeName wrapping a CallExpr.
	e1 := mustParseExpr(t, "arr(i)'length")
	an1, ok := e1.(*AttributeName)
	if !ok {
		t.Fatalf("case1: expected *AttributeName, got %T: %#v", e1, e1)
	}
	if _, ok := an1.X.(*CallExpr); !ok {
		t.Fatalf("case1: expected X to be *CallExpr, got %T", an1.X)
	}
	if an1.Attr != "length" {
		t.Fatalf("case1: expected Attr=length, got %q", an1.Attr)
	}
	// round-trip
	var b1 strings.Builder
	printExpr(&b1, e1)
	if !equalAST(e1, mustParseExpr(t, b1.String())) {
		t.Fatalf("case1: not AST-stable across print/reparse; printed: %q", b1.String())
	}

	// Case 2: dr_tmp(i)'LAST_EVENT inside an aggregate — the AttributeName node
	// is reachable from the aggregate.
	src2 := `package p is
constant C : time := (0 => dr_tmp(i)'LAST_EVENT);
end package;`
	df2, errs2 := ParseFile(NewFileSet(), "t.vhd", []byte(src2))
	if errs2 != nil {
		t.Fatalf("case2: parse errors: %v", errs2)
	}
	// Check that an AttributeName node exists somewhere in the tree.
	found := false
	Inspect(df2, func(n Node) bool {
		if _, ok := n.(*AttributeName); ok {
			found = true
		}
		return !found
	})
	if !found {
		t.Fatal("case2: no *AttributeName found in AST")
	}
	// round-trip
	out2 := Print(df2)
	df2b, errs2b := ParseFile(NewFileSet(), "t.vhd", []byte(out2))
	if errs2b != nil {
		t.Fatalf("case2: reparse errors: %v\n--- printed ---\n%s", errs2b, out2)
	}
	if !equalAST(df2, df2b) {
		t.Fatalf("case2: AST not stable across print/reparse\n--- printed ---\n%s", out2)
	}

	// Regression 1: s'range is still a flat Ident, NOT an AttributeName.
	e3 := mustParseExpr(t, "s'range")
	id3, ok := e3.(*Ident)
	if !ok || id3.Name != "s'range" {
		t.Fatalf("regression1: expected Ident{s'range}, got %T %#v", e3, e3)
	}
	var b3 strings.Builder
	printExpr(&b3, e3)
	if !equalAST(e3, mustParseExpr(t, b3.String())) {
		t.Fatalf("regression1: not AST-stable; printed: %q", b3.String())
	}

	// Regression 2: f(x) is still a plain CallExpr, not wrapped in AttributeName.
	e4 := mustParseExpr(t, "f(x)")
	if _, ok := e4.(*CallExpr); !ok {
		t.Fatalf("regression2: expected *CallExpr, got %T", e4)
	}
	var b4 strings.Builder
	printExpr(&b4, e4)
	if !equalAST(e4, mustParseExpr(t, b4.String())) {
		t.Fatalf("regression2: not AST-stable; printed: %q", b4.String())
	}
}

// roundTripWithOpts parses src with filename and opts, asserts zero errors,
// prints, reparses (no opts), and checks AST equality.
func roundTripWithOpts(t *testing.T, filename, src string, opts ...Option) *DesignFile {
	t.Helper()
	df, errs := ParseFile(NewFileSet(), filename, []byte(src), opts...)
	if errs != nil {
		t.Fatalf("parse errors: %v", errs)
	}
	out := Print(df)
	df2, errs2 := ParseFile(NewFileSet(), filename, []byte(out))
	if errs2 != nil {
		t.Fatalf("reparse errors: %v\n--- printed ---\n%s", errs2, out)
	}
	if !equalAST(df, df2) {
		t.Fatalf("AST not stable across print/reparse\n--- printed ---\n%s", out)
	}
	return df
}

func TestParseWithCPPDefine(t *testing.T) {
	if _, err := exec.LookPath("gcc"); err != nil {
		t.Skip("gcc not available")
	}

	// Sub-case 1: #if FOO with WithCPPDefine("FOO","1") → entity e (NOT wrong).
	t.Run("if_value_define", func(t *testing.T) {
		src := "#if FOO\nentity e is end entity;\n#else\nentity wrong is end entity;\n#endif\n"
		df := roundTripWithOpts(t, "t.vhd", src, WithCPP("gcc"), WithCPPDefine("FOO", "1"))
		if len(df.Units) != 1 {
			t.Fatalf("expected 1 unit, got %d", len(df.Units))
		}
		ent, ok := df.Units[0].(*EntityDecl)
		if !ok {
			t.Fatalf("expected *EntityDecl, got %T", df.Units[0])
		}
		if ent.Name != "e" {
			t.Fatalf("expected entity name e, got %q (FOO=1 branch should be taken)", ent.Name)
		}
	})

	// Sub-case 2: #ifdef FOO with WithCPPDefine("FOO","") → entity e present.
	t.Run("ifdef_bare_define", func(t *testing.T) {
		src := "#ifdef FOO\nentity e is end entity;\n#endif\n"
		df := roundTripWithOpts(t, "t.vhd", src, WithCPP("gcc"), WithCPPDefine("FOO", ""))
		if len(df.Units) != 1 {
			t.Fatalf("expected 1 unit, got %d", len(df.Units))
		}
		ent, ok := df.Units[0].(*EntityDecl)
		if !ok {
			t.Fatalf("expected *EntityDecl, got %T", df.Units[0])
		}
		if ent.Name != "e" {
			t.Fatalf("expected entity name e, got %q", ent.Name)
		}
	})
}

func TestParseWithCPPInclude(t *testing.T) {
	if _, err := exec.LookPath("gcc"); err != nil {
		t.Skip("gcc not available")
	}

	tmp := t.TempDir()
	subDir := filepath.Join(tmp, "sub")
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatalf("mkdir sub: %v", err)
	}
	incFile := filepath.Join(subDir, "inc.vhd")
	if err := os.WriteFile(incFile, []byte("entity inc_e is end entity;\n"), 0o644); err != nil {
		t.Fatalf("write inc.vhd: %v", err)
	}

	mainFilename := filepath.Join(tmp, "main.vhd")
	src := "#include \"inc.vhd\"\n"
	df := roundTripWithOpts(t, mainFilename, src, WithCPP("gcc"), WithCPPInclude("sub"))
	if len(df.Units) != 1 {
		t.Fatalf("expected 1 unit, got %d", len(df.Units))
	}
	ent, ok := df.Units[0].(*EntityDecl)
	if !ok {
		t.Fatalf("expected *EntityDecl, got %T", df.Units[0])
	}
	if ent.Name != "inc_e" {
		t.Fatalf("expected entity name inc_e, got %q", ent.Name)
	}
}

// TestProbeCpuTb is an informational probe: it tries to parse cpu_tb.vhd with
// the build defines and reports the round-trip result. It NEVER fails the suite.
func TestProbeCpuTb(t *testing.T) {
	root := os.Getenv("JCORE_SOC_ROOT")
	if root == "" {
		t.Skip("JCORE_SOC_ROOT not set")
	}
	if _, err := exec.LookPath("gcc"); err != nil {
		t.Skip("gcc not available")
	}
	tbPath := filepath.Join(root, "components/cpu/sim/cpu_tb.vhd")
	src, err := os.ReadFile(tbPath)
	if err != nil {
		t.Skipf("cpu_tb.vhd not found: %v", err)
	}

	opts := []Option{
		WithCPP("gcc"),
		WithCPPDefine("VHDL", ""),
		WithCPPDefine("CONFIG_PREFETCHER", "0"),
		WithCPPDefine("CONFIG_RING_BUS", "0"),
		WithCPPInclude("sim"),
	}

	df1, errs1 := ParseFile(NewFileSet(), tbPath, src, opts...)
	if errs1 != nil {
		t.Logf("PROBE: cpu_tb.vhd parse errors (does NOT round-trip): first error: %v", errutil.Errors(errs1)[0])
		return
	}
	out := Print(df1)
	df2, errs2 := ParseFile(NewFileSet(), tbPath, []byte(out))
	if errs2 != nil {
		t.Logf("PROBE: cpu_tb.vhd does NOT round-trip; first reparse error: %v", errutil.Errors(errs2)[0])
		return
	}
	if !equalAST(df1, df2) {
		t.Logf("PROBE: cpu_tb.vhd parses OK but AST not stable across print/reparse")
		return
	}
	t.Logf("PROBE: cpu_tb.vhd ROUND-TRIPS CLEANLY (parse+print+reparse+equalAST all pass)")
}
