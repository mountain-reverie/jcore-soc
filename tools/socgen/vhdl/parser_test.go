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

func TestDeferredUnitTagged(t *testing.T) {
	_, errs := parse(t, "architecture a of e is\nbegin\nend architecture;")
	if len(errs) == 0 {
		t.Fatal("expected a deferred-unit error")
	}
	found := false
	for _, e := range errs {
		if strings.Contains(e.Error(), "P1a") {
			found = true
		}
	}
	if !found {
		t.Fatalf("want a P1a-tagged deferred error, got %v", errs)
	}
}
