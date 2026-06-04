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

	// malformed entity class (not a reserved word) must be rejected.
	_, errs3 := ParseFile(NewFileSet(), "t.vhd", []byte("package p is\nattribute foo of x : notakeyword is 1;\nend package;"))
	if len(errs3) == 0 {
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
	// An entity statement part (passive statements after `begin`) is still
	// deferred; it must be tagged with a "deferred" error so the file is excluded.
	_, errs := parse(t, "entity e is\nbegin\n  assert true;\nend entity;")
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

func TestParseArchitectureSimple(t *testing.T) {
	src := `architecture rtl of e is
  signal x : std_logic;
begin
  x <= a;
  y <= a and b;
end architecture rtl;`
	df, errs := ParseFile(NewFileSet(), "t.vhd", []byte(src))
	if len(errs) != 0 {
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
	if len(errs2) != 0 || !equalAST(df, df2) {
		t.Fatalf("architecture not AST-stable: errs=%v\n%s", errs2, out)
	}
}


func TestParseConditionalAssign(t *testing.T) {
	src := `architecture rtl of e is
begin
  q <= a when sel = '1' else b;
  r <= x when c1 else y when c2 else z;
  s <= d;
end architecture;`
	df, errs := ParseFile(NewFileSet(), "t.vhd", []byte(src))
	if len(errs) != 0 {
		t.Fatalf("errs: %v", errs)
	}
	arch := df.Units[0].(*ArchitectureBody)
	q := arch.Stmts[0].(*ConcurrentSignalAssign)
	if q.Waveform != nil || len(q.Conds) != 2 {
		t.Fatalf("q conds: %#v", q)
	}
	if q.Conds[0].Cond == nil || q.Conds[1].Cond != nil { // last arm is the bare else
		t.Fatalf("q cond shape: %#v", q.Conds)
	}
	r := arch.Stmts[1].(*ConcurrentSignalAssign)
	if len(r.Conds) != 3 {
		t.Fatalf("r conds: %d", len(r.Conds))
	}
	s := arch.Stmts[2].(*ConcurrentSignalAssign)
	if s.Waveform == nil || len(s.Conds) != 0 { // simple assignment unchanged
		t.Fatalf("s simple: %#v", s)
	}
	// round-trip
	out := Print(df)
	df2, errs2 := ParseFile(NewFileSet(), "t.vhd", []byte(out))
	if len(errs2) != 0 || !equalAST(df, df2) {
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
	if len(errs) != 0 {
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
	if len(errs2) != 0 || !equalAST(df, df2) {
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
	if len(errs) != 0 {
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
	if len(errs2) != 0 || !equalAST(df, df2) {
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
	if len(errs) != 0 {
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
	if len(errs2) != 0 || !equalAST(df, df2) {
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
	if len(errs) != 0 {
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
	if len(errs2) != 0 || !equalAST(df, df2) {
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
	if len(errs) != 0 {
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
	if len(errs2) != 0 || !equalAST(df, df2) {
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
	if len(errs) != 0 {
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
	if len(errs2) != 0 || !equalAST(df, df2) {
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
	if len(errs) != 0 {
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
	if len(errs2) != 0 || !equalAST(df, df2) {
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
	if len(errs) != 0 {
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
	if len(errs2) != 0 || !equalAST(df, df2) {
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
	if len(errs) != 0 {
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
	if len(errs2) != 0 || !equalAST(df, df2) {
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
	if len(errs) != 0 {
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
	if len(errs2) != 0 || !equalAST(df, df2) {
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
	if len(errs) != 0 {
		t.Fatalf("errs: %v", errs)
	}
	out := Print(df)
	df2, errs2 := ParseFile(NewFileSet(), "t.vhd", []byte(out))
	if len(errs2) != 0 || !equalAST(df, df2) {
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
	if len(errs) != 0 {
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
	if len(errs2) != 0 || !equalAST(df, df2) {
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
	if len(errs) != 0 {
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
	if len(errs2) != 0 {
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
		if len(e) != 0 || len(e2) != 0 || !equalAST(d, d2) {
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
	if len(errs) != 0 {
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
	if len(errs2) != 0 || !equalAST(df, df2) {
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
	if len(errs) != 0 {
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
	if len(errs2) != 0 || !equalAST(df, df2) {
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
	if len(errs) != 0 {
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
	if len(errs2) != 0 || !equalAST(df, df2) {
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
	if len(errs) != 0 {
		t.Fatalf("errs: %v", errs)
	}
	out := Print(df)
	df2, errs2 := ParseFile(NewFileSet(), "t.vhd", []byte(out))
	if len(errs2) != 0 || !equalAST(df, df2) {
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
	if len(errs) != 0 {
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
	if len(errs2) != 0 || !equalAST(df, df2) {
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
	if len(errs) != 0 {
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
	if len(errs2) != 0 || !equalAST(df, df2) {
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
	if len(errs) != 0 {
		t.Fatalf("errs: %v", errs)
	}
	out := Print(df)
	df2, errs2 := ParseFile(NewFileSet(), "t.vhd", []byte(out))
	if len(errs2) != 0 || !equalAST(df, df2) {
		t.Fatalf("file/access/use not AST-stable: errs=%v\n%s", errs2, out)
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
	if len(errs) != 0 {
		t.Fatalf("errs: %v", errs)
	}
	out := Print(df)
	df2, errs2 := ParseFile(NewFileSet(), "t.vhd", []byte(out))
	if len(errs2) != 0 || !equalAST(df, df2) {
		t.Fatalf("suffix-chain name not AST-stable: errs=%v\n%s", errs2, out)
	}
}
