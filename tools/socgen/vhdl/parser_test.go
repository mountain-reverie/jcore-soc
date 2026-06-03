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
	_, errs := parse(t, "configuration c of e is\nend configuration;")
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

func TestParseProcessWithWaitDeferred(t *testing.T) {
	src := `architecture rtl of e is
begin
  process begin
    wait until clk = '1';
  end process;
end architecture;`
	_, errs := ParseFile(NewFileSet(), "t.vhd", []byte(src))
	if len(errs) == 0 {
		t.Fatal("expected a deferred error for a wait statement")
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

func TestParseProcessWithWhileDeferred(t *testing.T) {
	// while-loops stay deferred in P1d-1.
	src := `architecture rtl of e is
begin
  process begin
    while x loop
      x := false;
    end loop;
  end process;
end architecture;`
	_, errs := ParseFile(NewFileSet(), "t.vhd", []byte(src))
	if len(errs) == 0 {
		t.Fatal("expected a deferred error for a while-loop")
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
