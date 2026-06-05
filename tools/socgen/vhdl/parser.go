package vhdl

import (
	"fmt"
	"strings"
)

// parser holds the token stream and error list for recursive-descent parsing.
type parser struct {
	toks []Token
	i    int
	errs []error
	file *File
}

// newParser lexes all tokens from src, dropping COMMENT tokens, and returns a
// parser ready to consume the stream. The final EOF token is retained.
func newParser(src []byte, f *File) *parser {
	if f == nil {
		f = NewFileSet().AddFile("", len(src))
	}
	l := NewLexer(src, f)
	toks := make([]Token, 0, len(src)/3+8) // rough capacity hint; minimise re-alloc
	// Hard progress bound: every non-EOF token consumes >=1 source byte, so the
	// total number of Next() calls cannot exceed len(src)+1. The cap is a
	// belt-and-suspenders against a lexer that ever fails to advance — without it
	// such a bug would build an unbounded token slice and exhaust memory.
	maxIters := len(src)*2 + 16
	for iters := 0; ; iters++ {
		if iters > maxIters {
			toks = append(toks, Token{Kind: EOF})
			break
		}
		tok := l.Next()
		if tok.Kind == COMMENT {
			continue
		}
		toks = append(toks, tok)
		if tok.Kind == EOF {
			break
		}
	}
	return &parser{toks: toks, file: f}
}

// newParserFromExpr is a test seam for parsing a bare expression.
func newParserFromExpr(src []byte) *parser {
	return newParser(src, nil)
}

// errorf records a parse error at the given source position, rendered through
// the file's position table (file:line:col).
func (p *parser) errorf(at Pos, format string, args ...any) {
	p.errs = append(p.errs, fmt.Errorf("%s: "+format, append([]any{p.file.Position(at)}, args...)...))
}

// cur returns the current token. If the index is past the end of the slice
// (shouldn't normally happen because the EOF token is always retained), it
// returns the last token in the slice (which is EOF).
func (p *parser) cur() Token {
	if p.i >= len(p.toks) {
		return p.toks[len(p.toks)-1]
	}
	return p.toks[p.i]
}

// at reports whether the current token has kind k.
func (p *parser) at(k Kind) bool {
	return p.cur().Kind == k
}

// advance consumes the current token and returns it.
func (p *parser) advance() Token {
	tok := p.cur()
	if p.i < len(p.toks)-1 {
		p.i++
	}
	return tok
}

// accept advances past the current token if its kind is k, returning true.
func (p *parser) accept(k Kind) bool {
	if p.at(k) {
		p.advance()
		return true
	}
	return false
}

// expect advances and returns the current token if its kind is k. Otherwise it
// records an error and returns the current token without advancing.
func (p *parser) expect(k Kind) Token {
	tok := p.cur()
	if tok.Kind == k {
		p.advance()
		return tok
	}
	p.errorf(tok.Pos, "expected %v, got %v %q", k, tok.Kind, tok.Lit)
	return tok
}

// peekKind returns the Kind of the token n positions ahead of the current one,
// or EOF if that position is past the end of the token stream.
func (p *parser) peekKind(n int) Kind {
	i := p.i + n
	if i >= len(p.toks) {
		return EOF
	}
	return p.toks[i].Kind
}

// ensureProgress guarantees a parsing loop makes forward progress: if the token
// index has not advanced past start (and we are not at EOF), it records a
// recovery error and force-advances one token. Every loop that calls a
// sub-parser which could consume zero tokens on malformed input MUST call this,
// so the parser can never spin (which would grow errs unbounded and OOM).
func (p *parser) ensureProgress(start int, where string) {
	if p.i == start && !p.at(EOF) {
		t := p.cur()
		p.errorf(t.Pos, "skipping unexpected %v %q in %s", t.Kind, t.Lit, where)
		p.advance()
	}
}

// The expression grammar is a VHDL-93 precedence-climbing tier ladder. From
// lowest to highest precedence: logical < relational < shift < adding (simple) <
// multiplying (term) < (sign/abs/not + **) (factor) < primary. Each tier
// left-folds its operator set into LEFT-ASSOCIATIVE BinaryExpr nodes and
// delegates to the next-higher tier. Every fold loop calls advance() on the
// operator token before recursing, so each iteration consumes ≥1 token and the
// loop provably terminates — no runaway risk.

// parseExpr is the expression entry point: the logical tier, then an optional
// range direction suffix (to/downto).
func (p *parser) parseExpr() Expr {
	left := p.parseLogical()
	if p.at(TO) || p.at(DOWNTO) {
		dir := p.advance()
		right := p.parseLogical()
		return &Range{Left: left, DirPos: dir.Pos, Dir: dir.Kind, Right: right}
	}
	return left
}

// parseDiscreteRange parses a range that may carry a type mark:
// `integer range 0 to 7`, or a plain range/attribute like `0 to 7`.
func (p *parser) parseDiscreteRange() Expr {
	e := p.parseExpr()
	if p.accept(RANGE) {
		return &RangeConstraint{P: e.Pos(), Mark: e, Range: p.parseExpr()}
	}
	return e
}

// parseLogical handles the lowest-precedence logical operators.
func (p *parser) parseLogical() Expr {
	left := p.parseRelation()
	for {
		switch p.cur().Kind {
		case AND, OR, NAND, NOR, XOR, XNOR:
			op := p.advance()
			right := p.parseRelation()
			left = &BinaryExpr{X: left, OpPos: op.Pos, Op: op.Kind, Y: right}
		default:
			return left
		}
	}
}

// parseRelation parses the relational tier (= /= < <= > >=). VHDL-93 LRM 7.2
// permits at most one relational operator per expression; this loop is
// permissive (it would accept a = b = c) but parses all valid input correctly.
// TODO(strict): a one-shot `if` would enforce the LRM cardinality.
func (p *parser) parseRelation() Expr {
	left := p.parseShift()
	for {
		switch p.cur().Kind {
		case EQ, NE, LT, LE, GT, GE:
			op := p.advance()
			right := p.parseShift()
			left = &BinaryExpr{X: left, OpPos: op.Pos, Op: op.Kind, Y: right}
		default:
			return left
		}
	}
}

// parseShift parses the shift/rotate tier (sll srl sla sra rol ror). VHDL-93
// LRM 7.3.2 permits at most one shift operator per expression; this loop is
// permissive but parses all valid input correctly.
func (p *parser) parseShift() Expr {
	left := p.parseSimple()
	for {
		switch p.cur().Kind {
		case SLL, SRL, SLA, SRA, ROL, ROR:
			op := p.advance()
			right := p.parseSimple()
			left = &BinaryExpr{X: left, OpPos: op.Pos, Op: op.Kind, Y: right}
		default:
			return left
		}
	}
}

// parseSimple handles a simple expression: an optional leading sign followed by
// terms joined by adding operators (+ - &).
func (p *parser) parseSimple() Expr {
	var left Expr
	if p.at(PLUS) || p.at(MINUS) {
		op := p.advance()
		left = &UnaryExpr{OpPos: op.Pos, Op: op.Kind, X: p.parseTerm()}
	} else {
		left = p.parseTerm()
	}
	for {
		switch p.cur().Kind {
		case PLUS, MINUS, AMP:
			op := p.advance()
			right := p.parseTerm()
			left = &BinaryExpr{X: left, OpPos: op.Pos, Op: op.Kind, Y: right}
		default:
			return left
		}
	}
}

// parseTerm handles multiplying operators.
func (p *parser) parseTerm() Expr {
	left := p.parseFactor()
	for {
		switch p.cur().Kind {
		case STAR, SLASH, MOD, REM:
			op := p.advance()
			right := p.parseFactor()
			left = &BinaryExpr{X: left, OpPos: op.Pos, Op: op.Kind, Y: right}
		default:
			return left
		}
	}
}

// parseFactor handles abs/not unary operators (each applied to one primary)
// and the non-associative ** operator (primary ** primary). Both bind more
// tightly than any multiplying operator. Per VHDL-93 LRM 7.3.4 the operand of
// abs/not is a primary (NOT another factor), and ** is non-associative — so
// abs/not do not nest and a ** b ** c is not folded without parens. This is
// intentional; do not change parsePrimary() to parseFactor() here.
func (p *parser) parseFactor() Expr {
	if p.at(ABS) || p.at(NOT) {
		op := p.advance()
		return &UnaryExpr{OpPos: op.Pos, Op: op.Kind, X: p.parsePrimary()}
	}
	left := p.parsePrimary()
	if p.at(EXP) {
		op := p.advance()
		right := p.parsePrimary()
		return &BinaryExpr{X: left, OpPos: op.Pos, Op: op.Kind, Y: right}
	}
	return left
}

// parsePrimary parses a primary expression: literal, parenthesized, or
// name/call-or-index.
func (p *parser) parsePrimary() Expr {
	tok := p.cur()

	switch tok.Kind {
	case INT, REAL, BASEDLIT:
		p.advance()
		if p.at(IDENT) {
			// physical literal: abstract literal + unit name
			unit := p.advance().Lit
			return &PhysicalLit{ValuePos: tok.Pos, Value: tok.Lit, Unit: unit}
		}
		return &BasicLit{ValuePos: tok.Pos, Kind: tok.Kind, Value: tok.Lit}
	case CHARLIT, STRINGLIT, BITSTRINGLIT:
		p.advance()
		return &BasicLit{ValuePos: tok.Pos, Kind: tok.Kind, Value: tok.Lit}

	case LPAREN:
		return p.parseParenOrAggregate()

	case IDENT, EXTIDENT:
		return p.parseName()

	default:
		// Keywords that can appear as primaries in expressions (e.g. inside
		// aggregates: others, all; attribute names: range, etc.)
		switch tok.Kind {
		case NEW:
			npos := p.advance().Pos
			return &AllocatorExpr{New: npos, X: p.parseExpr()}
		case OTHERS, ALL, RANGE, NULL, OPEN:
			p.advance()
			return &Ident{NamePos: tok.Pos, Name: tok.Kind.String()}
		}
		// Unrecognised primary — record an error and return an empty literal so
		// the caller can continue.
		p.errorf(tok.Pos, "unexpected token %v %q in primary", tok.Kind, tok.Lit)
		return &BasicLit{ValuePos: tok.Pos, Kind: tok.Kind, Value: ""}
	}
}

// parseParenOrAggregate parses a parenthesized construct: either a single
// parenthesized expression -> *ParenExpr, or an element-association list ->
// *Aggregate. The decision: exactly one positional element (no `=>`) is a
// ParenExpr; anything else (named element, or >1 element) is an Aggregate. Zero
// elements (empty "()") also produces an empty *Aggregate.
func (p *parser) parseParenOrAggregate() Expr {
	open := p.expect(LPAREN)
	var elems []*ElementAssoc
	if !p.at(RPAREN) {
		elems = append(elems, p.parseElementAssoc())
		for p.accept(COMMA) { // each iteration consumes the comma -> always advances
			if p.at(RPAREN) {
				break // tolerate a trailing comma defensively
			}
			elems = append(elems, p.parseElementAssoc())
		}
	}
	closeTok := p.expect(RPAREN)
	if len(elems) == 1 && elems[0].Choices == nil {
		return &ParenExpr{Lparen: open.Pos, X: elems[0].X, Rparen: closeTok.Pos}
	}
	return &Aggregate{Lparen: open.Pos, Elems: elems, Rparen: closeTok.Pos}
}

// parseElementAssoc parses one aggregate element: either a positional expr, or
// `choice {| choice} => expr`.
func (p *parser) parseElementAssoc() *ElementAssoc {
	first := p.parseExpr()
	if p.at(BAR) || p.at(ARROW) {
		choices := []Expr{first}
		for p.accept(BAR) { // each iteration consumes a '|' -> always advances
			choices = append(choices, p.parseExpr())
		}
		arrow := p.expect(ARROW)
		return &ElementAssoc{Choices: choices, ArrowPos: arrow.Pos, X: p.parseExpr()}
	}
	return &ElementAssoc{X: first} // positional (Choices == nil)
}

// parseFile parses a complete VHDL design file: context clauses (library/use)
// and design units may appear in any order at the top level.
func (p *parser) parseFile() *DesignFile {
	df := &DesignFile{}

	for !p.at(EOF) {
		start := p.i
		switch p.cur().Kind {
		case LIBRARY:
			pos := p.advance().Pos // consume LIBRARY
			names := []string{p.expect(IDENT).Lit}
			for p.accept(COMMA) {
				names = append(names, p.expect(IDENT).Lit)
			}
			p.expect(SEMICOLON)
			df.Context = append(df.Context, &LibraryClause{P: pos, Names: names})
		case USE:
			df.Context = append(df.Context, p.parseUseClause())
		case PACKAGE:
			if p.peekKind(1) == BODY {
				if u := p.parsePackageBody(); u != nil {
					df.Units = append(df.Units, u)
				}
			} else {
				if u := p.parsePackageDecl(); u != nil {
					df.Units = append(df.Units, u)
				}
			}
		case ENTITY:
			u := p.parseEntityDecl()
			if u != nil {
				df.Units = append(df.Units, u)
			}
		case ARCHITECTURE:
			u := p.parseArchitectureBody()
			if u != nil {
				df.Units = append(df.Units, u)
			}
		case CONFIGURATION:
			if u := p.parseConfigurationDecl(); u != nil {
				df.Units = append(df.Units, u)
			}
		default:
			p.errorf(p.cur().Pos, "unexpected token %v %q at top level", p.cur().Kind, p.cur().Lit)
			return df
		}
		p.ensureProgress(start, "design unit")
	}
	return df
}

// parseConfig holds options for ParseFile.
type parseConfig struct {
	cpp         string
	cppDefines  []cppDefine // -Dkey[=value], in order
	cppIncludes []string    // extra include dirs, resolved relative to the source file's dir
}

// cppDefine represents a single -D flag for the C preprocessor.
type cppDefine struct{ key, value string }

// Option configures ParseFile.
type Option func(*parseConfig)

// WithCPP runs exe (e.g. "gcc") as a C preprocessor over the source before
// lexing, using jcore's common.mk flags. Only the executable name is
// configurable.
func WithCPP(exe string) Option { return func(c *parseConfig) { c.cpp = exe } }

// WithCPPDefine adds a `-Dkey=value` (or `-Dkey` when value is "") to the
// preprocessor invocation. Only effective alongside WithCPP.
func WithCPPDefine(key, value string) Option {
	return func(c *parseConfig) { c.cppDefines = append(c.cppDefines, cppDefine{key, value}) }
}

// WithCPPInclude adds an extra preprocessor include directory, resolved
// relative to the source file's directory (like the standing -I<dir>/config).
// Only effective alongside WithCPP.
func WithCPPInclude(dir string) Option {
	return func(c *parseConfig) { c.cppIncludes = append(c.cppIncludes, dir) }
}

// ParseFile parses src (named filename) into a DesignFile. fset records source
// positions; returned errors are rendered via fset.Position. A nil or empty
// error slice means a clean parse.
func ParseFile(fset *FileSet, filename string, src []byte, opts ...Option) (*DesignFile, []error) {
	var cfg parseConfig
	for _, o := range opts {
		o(&cfg)
	}
	if cfg.cpp != "" {
		out, err := runCPP(cfg, filename, src)
		if err != nil {
			return nil, []error{fmt.Errorf("%s: cpp (%s) failed: %w", filename, cfg.cpp, err)}
		}
		src = out
	}
	f := fset.AddFile(filename, len(src))
	p := newParser(src, f)
	df := p.parseFile()
	return df, p.errs
}

// parseDottedName reads a possibly-dotted name (e.g. ieee.std_logic_1164.all)
// and returns it as a string.
func (p *parser) parseDottedName() string {
	tok := p.cur()
	var text string
	if tok.Kind == IDENT || tok.Kind == EXTIDENT {
		p.advance()
		text = tok.Lit
	} else if tok.Kind > kwStart && tok.Kind < kwEnd {
		p.advance()
		text = tok.Kind.String()
	} else {
		p.errorf(tok.Pos, "expected name, got %v %q", tok.Kind, tok.Lit)
		return ""
	}
	for p.at(DOT) {
		p.advance() // consume '.'
		next := p.cur()
		if next.Kind == IDENT || next.Kind == EXTIDENT || next.Kind == ALL {
			p.advance()
			seg := next.Lit
			if seg == "" {
				seg = next.Kind.String()
			}
			text += "." + seg
		} else if next.Kind > kwStart && next.Kind < kwEnd {
			p.advance()
			text += "." + next.Kind.String()
		} else {
			break
		}
	}
	return text
}

// parseUseClause parses `use name {, name} ;`.
func (p *parser) parseUseClause() *UseClause {
	pos := p.expect(USE).Pos
	names := []string{p.parseDottedName()}
	for p.accept(COMMA) {
		names = append(names, p.parseDottedName())
	}
	p.expect(SEMICOLON)
	return &UseClause{P: pos, Names: names}
}

// parseConfigurationDecl parses `configuration name of entity is <uses> <block>
// end [configuration] [name] ;`.
func (p *parser) parseConfigurationDecl() *ConfigurationDecl {
	pos := p.expect(CONFIGURATION).Pos
	name := p.expect(IDENT).Lit
	p.expect(OF)
	entity := p.parseDottedName()
	p.expect(IS)
	var decls []*UseClause
	for p.at(USE) {
		decls = append(decls, p.parseUseClause())
	}
	var block *BlockConfig
	if p.at(FOR) {
		if b, ok := p.parseConfigItem().(*BlockConfig); ok {
			block = b
		}
	}
	p.expect(END)
	p.accept(CONFIGURATION)
	if p.at(IDENT) {
		p.advance() // optional closing name
	}
	p.expect(SEMICOLON)
	return &ConfigurationDecl{P: pos, Name: name, Entity: entity, Decls: decls, Block: block}
}

// parseConfigItem parses one `for ...` configuration item: a block configuration
// (`for label <uses> <items> end for`) or a component configuration (`for
// inst_list : comp ...` — DEFERRED in Task 1).
func (p *parser) parseConfigItem() Node {
	pos := p.expect(FOR).Pos
	// spec list: name {, name}  (name = IDENT | all | others)
	specs := []string{p.parseConfigSpecName()}
	for p.accept(COMMA) {
		specs = append(specs, p.parseConfigSpecName())
	}
	if p.at(COLON) {
		// component configuration: `for inst_list : comp [binding;] [block] end for ;`
		p.advance() // consume COLON
		comp := p.parseDottedName()
		var binding *BindingIndication
		if p.at(USE) {
			binding = p.parseBindingIndication()
		}
		var block *BlockConfig
		if p.at(FOR) {
			if b, ok := p.parseConfigItem().(*BlockConfig); ok {
				block = b
			}
		}
		p.expect(END)
		p.expect(FOR)
		p.expect(SEMICOLON)
		return &ComponentConfig{P: pos, Insts: specs, Comp: comp, Binding: binding, Block: block}
	}
	// block configuration: specs[0] is the architecture/block/generate label.
	spec := ""
	if len(specs) > 0 {
		spec = specs[0]
	}
	var uses []*UseClause
	for p.at(USE) {
		uses = append(uses, p.parseUseClause())
	}
	var items []Node
	for !p.at(END) && !p.at(EOF) {
		start := p.i
		if it := p.parseConfigItem(); it != nil {
			items = append(items, it)
		}
		p.ensureProgress(start, "configuration item")
	}
	p.expect(END)
	p.expect(FOR)
	p.expect(SEMICOLON)
	return &BlockConfig{P: pos, Spec: spec, Uses: uses, Items: items}
}

// parseBindingIndication parses `use (entity name[(arch)] | configuration name |
// open) [generic map (...)] [port map (...)] ;`.
func (p *parser) parseBindingIndication() *BindingIndication {
	pos := p.expect(USE).Pos
	var kind Kind
	var unit, arch string
	switch p.cur().Kind {
	case ENTITY:
		p.advance()
		unit = p.parseDottedName()
		if p.at(LPAREN) {
			p.advance()
			arch = p.expect(IDENT).Lit
			p.expect(RPAREN)
		}
		kind = ENTITY
	case CONFIGURATION:
		p.advance()
		unit = p.parseDottedName()
		kind = CONFIGURATION
	case OPEN:
		p.advance()
		kind = OPEN
	default:
		t := p.cur()
		p.errorf(t.Pos, "expected entity/configuration/open in binding, got %v %q", t.Kind, t.Lit)
	}
	var gmap, pmap []*AssocElement
	if p.at(GENERIC) {
		p.advance()
		p.expect(MAP)
		gmap = p.parseAssocList()
	}
	if p.at(PORT) {
		p.advance()
		p.expect(MAP)
		pmap = p.parseAssocList()
	}
	p.expect(SEMICOLON)
	return &BindingIndication{P: pos, UnitKind: kind, Unit: unit, Arch: arch, GenericMap: gmap, PortMap: pmap}
}

// parseConfigSpecName reads one spec/instantiation name: an identifier or the
// keywords `all`/`others`.
func (p *parser) parseConfigSpecName() string {
	switch p.cur().Kind {
	case IDENT:
		return p.advance().Lit
	case ALL, OTHERS:
		return p.advance().Kind.String()
	default:
		t := p.cur()
		p.errorf(t.Pos, "expected configuration spec name, got %v %q", t.Kind, t.Lit)
		p.advance() // ensure progress
		return ""
	}
}

// parsePackageDecl parses: PACKAGE name IS {decl} END [PACKAGE] [name] ;
func (p *parser) parsePackageDecl() *PackageDecl {
	pos := p.expect(PACKAGE).Pos
	name := p.expect(IDENT).Lit
	p.expect(IS)

	var decls []Decl
	for !p.at(END) && !p.at(EOF) {
		start := p.i
		d := p.parseDecl()
		if d != nil {
			decls = append(decls, d)
		}
		p.ensureProgress(start, "package declaration")
	}
	p.expect(END)
	p.accept(PACKAGE)
	// optional closing name
	if p.at(IDENT) {
		p.advance()
	}
	p.expect(SEMICOLON)
	return &PackageDecl{P: pos, Name: name, Decls: decls}
}

// parsePackageBody parses `package body name is <decls> end [package body] [name] ;`.
func (p *parser) parsePackageBody() *PackageBody {
	pos := p.expect(PACKAGE).Pos
	p.expect(BODY)
	name := p.expect(IDENT).Lit
	p.expect(IS)
	var decls []Decl
	for !p.at(END) && !p.at(EOF) {
		start := p.i
		if d := p.parseDecl(); d != nil {
			decls = append(decls, d)
		}
		p.ensureProgress(start, "package body declaration")
	}
	p.expect(END)
	p.accept(PACKAGE)
	p.accept(BODY)
	if p.at(IDENT) {
		p.advance() // optional closing name
	}
	p.expect(SEMICOLON)
	return &PackageBody{P: pos, Name: name, Decls: decls}
}

// parseEntityDecl parses: ENTITY name IS [GENERIC(...);] [PORT(...);] END [ENTITY] [name] ;
func (p *parser) parseEntityDecl() *EntityDecl {
	pos := p.expect(ENTITY).Pos
	name := p.expect(IDENT).Lit
	p.expect(IS)

	var generics, ports []*InterfaceDecl
	if p.at(GENERIC) {
		p.advance() // consume GENERIC
		generics = p.parseInterfaceList()
		p.expect(SEMICOLON)
	}
	if p.at(PORT) {
		p.advance() // consume PORT
		ports = p.parseInterfaceList()
		p.expect(SEMICOLON)
	}

	// Entity declarative part: declarations until END/BEGIN/EOF.
	var decls []Decl
	for !p.at(END) && !p.at(BEGIN) && !p.at(EOF) {
		start := p.i
		if d := p.parseDecl(); d != nil {
			decls = append(decls, d)
		}
		p.ensureProgress(start, "entity declaration")
	}
	var stmts []Stmt
	if p.accept(BEGIN) {
		for !p.at(END) && !p.at(EOF) {
			start := p.i
			if s := p.parseConcurrentStmt(); s != nil {
				stmts = append(stmts, s)
			}
			p.ensureProgress(start, "entity statement")
		}
	}

	p.expect(END)
	p.accept(ENTITY)
	if p.at(IDENT) {
		p.advance()
	}
	p.expect(SEMICOLON)
	return &EntityDecl{P: pos, Name: name, Generics: generics, Ports: ports, Decls: decls, Stmts: stmts}
}

// parseArchitectureBody parses `architecture name of entity is <decls> begin
// <concurrent stmts> end [architecture] [name] ;`.
func (p *parser) parseArchitectureBody() *ArchitectureBody {
	pos := p.expect(ARCHITECTURE).Pos
	name := p.expect(IDENT).Lit
	p.expect(OF)
	entity := p.expect(IDENT).Lit
	p.expect(IS)
	// declarative part
	var decls []Decl
	for !p.at(BEGIN) && !p.at(END) && !p.at(EOF) {
		start := p.i
		if d := p.parseDecl(); d != nil {
			decls = append(decls, d)
		}
		p.ensureProgress(start, "architecture declaration")
	}
	p.expect(BEGIN)
	// concurrent statement part
	var stmts []Stmt
	for !p.at(END) && !p.at(EOF) {
		start := p.i
		if s := p.parseConcurrentStmt(); s != nil {
			stmts = append(stmts, s)
		}
		p.ensureProgress(start, "concurrent statement")
	}
	p.expect(END)
	p.accept(ARCHITECTURE)
	if p.at(IDENT) {
		p.advance()
	}
	p.expect(SEMICOLON)
	return &ArchitectureBody{P: pos, Name: name, Entity: entity, Decls: decls, Stmts: stmts}
}

// parseConcurrentStmt parses ONE concurrent statement. For P1c-1 Task 1 it
// supports only simple concurrent signal assignment ([label:] target <= expr ;);
// any other concurrent statement is deferred (file excluded). Later tasks extend
// the dispatch (instantiation, conditional assignment, generate).
func (p *parser) parseConcurrentStmt() Stmt {
	pos := p.cur().Pos
	label := ""
	// optional `label :` (an IDENT directly followed by COLON)
	if p.at(IDENT) && p.peekKind(1) == COLON {
		label = p.advance().Lit
		p.advance() // consume COLON
	}
	// statements introduced by a keyword (process/generate/block/...) are
	// deferred here; entity/component/configuration dispatch to instantiation.
	switch p.cur().Kind {
	case BLOCK, GENERATE:
		p.errorf(p.cur().Pos, "deferred: %v concurrent statement not yet parsed", p.cur().Kind)
		return nil
	case WITH:
		return p.parseSelectedAssign(pos, label)
	case ASSERT:
		return p.parseAssertStmt(pos, label)
	case POSTPONED, PROCESS:
		return p.parseProcess(pos, label)
	case FOR:
		return p.parseGenerate(pos, label, FOR)
	case IF:
		return p.parseGenerate(pos, label, IF)
	case ENTITY:
		p.advance()
		unit := p.parseDottedName()
		arch := ""
		if p.at(LPAREN) {
			p.advance()
			arch = p.expect(IDENT).Lit
			p.expect(RPAREN)
		}
		return p.finishInstantiation(pos, label, ENTITY, unit, arch)
	case COMPONENT:
		p.advance()
		return p.finishInstantiation(pos, label, COMPONENT, p.parseDottedName(), "")
	case CONFIGURATION:
		p.advance()
		return p.finishInstantiation(pos, label, CONFIGURATION, p.parseDottedName(), "")
	}
	// otherwise: a name. If `<=` follows, it's a simple concurrent signal
	// assignment; otherwise it may be a bare component instantiation.
	target := p.parseName()
	if p.at(LE) {
		p.advance() // consume '<='
		delay := p.parseDelayMechanism()
		wf := p.parseWaveform()
		if p.at(WHEN) {
			var conds []*CondWaveform
			p.advance() // consume WHEN
			conds = append(conds, &CondWaveform{Waveform: wf, Cond: p.parseExpr()})
			for p.accept(ELSE) { // each iteration consumes ELSE -> always advances
				w := p.parseWaveform()
				if p.at(WHEN) {
					p.advance()
					conds = append(conds, &CondWaveform{Waveform: w, Cond: p.parseExpr()})
				} else {
					conds = append(conds, &CondWaveform{Waveform: w, Cond: nil})
					break
				}
			}
			p.expect(SEMICOLON)
			return &ConcurrentSignalAssign{P: pos, Label: label, Target: target, Delay: delay, Conds: conds}
		}
		p.expect(SEMICOLON)
		return &ConcurrentSignalAssign{P: pos, Label: label, Target: target, Delay: delay, Waveform: wf}
	}
	// Concurrent procedure call: name(args) ;  (a call expression ending in ';')
	if _, ok := target.(*CallExpr); ok && p.at(SEMICOLON) {
		p.advance()
		return p.procCall(pos, label, target)
	}
	// Bare component instantiation: `label : comp_name [generic map][port map] ;`.
	// Only valid with a label and a simple name; otherwise defer.
	if label != "" {
		if id, ok := target.(*Ident); ok {
			return p.finishInstantiation(pos, label, 0, id.Name, "")
		}
	}
	p.errorf(p.cur().Pos, "deferred: concurrent statement not yet parsed")
	return nil
}

// procCall builds a ProcedureCallStmt from a parsed name/call target. Positional
// CallExpr.Args are already AssocElements (named or positional); a bare Ident is
// a parameterless call.
func (p *parser) procCall(pos Pos, label string, target Expr) Stmt {
	switch t := target.(type) {
	case *CallExpr:
		return &ProcedureCallStmt{P: pos, Label: label, Name: exprString(t.Fun), Args: t.Args}
	case *Ident:
		return &ProcedureCallStmt{P: pos, Label: label, Name: t.Name}
	default:
		p.errorf(pos, "expected procedure call, got %T", target)
		return nil
	}
}

// parseDelayMechanism consumes an optional signal-assignment delay mechanism
// preceding a waveform: `transport`, `inertial`, or `reject <expr> inertial`.
// Returns nil when none is present.
func (p *parser) parseDelayMechanism() *DelayMechanism {
	if p.accept(TRANSPORT) {
		return &DelayMechanism{Transport: true}
	}
	if p.accept(REJECT) {
		rej := p.parseExpr()
		p.expect(INERTIAL)
		return &DelayMechanism{Reject: rej}
	}
	if p.accept(INERTIAL) {
		return &DelayMechanism{}
	}
	return nil
}

// parseWaveformElem parses `value [after time]`.
func (p *parser) parseWaveformElem() *WaveformElem {
	v := p.parseExpr()
	var after Expr
	if p.accept(AFTER) {
		after = p.parseExpr()
	}
	return &WaveformElem{Value: v, After: after}
}

// parseWaveform parses `element {, element}`.
func (p *parser) parseWaveform() []*WaveformElem {
	wf := []*WaveformElem{p.parseWaveformElem()}
	for p.accept(COMMA) { // each iteration consumes COMMA -> always advances
		wf = append(wf, p.parseWaveformElem())
	}
	return wf
}

// parseSelectedAssign parses `with expr select target <= waveform when choices
// {, waveform when choices} ;`.
func (p *parser) parseSelectedAssign(pos Pos, label string) Stmt {
	p.expect(WITH)
	sel := p.parseExpr()
	p.expect(SELECT)
	target := p.parseName()
	p.expect(LE) // <=
	delay := p.parseDelayMechanism()
	var alts []*SelectedWaveform
	for {
		wf := p.parseWaveform() // stops at `when` (when is not a `,`)
		p.expect(WHEN)
		choices := p.parseChoices()
		alts = append(alts, &SelectedWaveform{Waveform: wf, Choices: choices})
		if !p.accept(COMMA) {
			break
		}
	}
	p.expect(SEMICOLON)
	return &SelectedSignalAssign{P: pos, Label: label, Expr: sel, Target: target, Delay: delay, Alts: alts}
}

// parseGenerate parses a for/if generate statement (label already consumed).
//
//	for id in range generate [decls begin] stmts end generate [label] ;
//	if cond generate          [decls begin] stmts end generate [label] ;
func (p *parser) parseGenerate(pos Pos, label string, kind Kind) Stmt {
	var param string
	var rng, cond Expr
	if kind == FOR {
		p.advance() // consume FOR
		param = p.expect(IDENT).Lit
		p.expect(IN)
		rng = p.parseDiscreteRange()
	} else {
		p.advance() // consume IF
		cond = p.parseExpr()
	}
	p.expect(GENERATE)
	// optional declarative part (present iff a `begin` follows it)
	var decls []Decl
	for isDeclStart(p.cur().Kind) {
		start := p.i
		if d := p.parseDecl(); d != nil {
			decls = append(decls, d)
		}
		p.ensureProgress(start, "generate declaration")
	}
	p.accept(BEGIN) // consume `begin` if present
	// concurrent statement part (recursive)
	var stmts []Stmt
	for !p.at(END) && !p.at(EOF) {
		start := p.i
		if s := p.parseConcurrentStmt(); s != nil {
			stmts = append(stmts, s)
		}
		p.ensureProgress(start, "generate statement")
	}
	p.expect(END)
	p.expect(GENERATE)
	if p.at(IDENT) {
		p.advance() // optional closing label
	}
	p.expect(SEMICOLON)
	return &GenerateStmt{P: pos, Label: label, Kind: kind, Param: param, Range: rng, Cond: cond, Decls: decls, Stmts: stmts}
}

// parseProcess parses a process statement (label already consumed).
func (p *parser) parseProcess(pos Pos, label string) Stmt {
	postponed := p.accept(POSTPONED)
	p.expect(PROCESS)
	var sens []Expr
	if p.at(LPAREN) {
		p.advance()
		if !p.at(RPAREN) {
			sens = append(sens, p.parseName())
			for p.accept(COMMA) {
				sens = append(sens, p.parseName())
			}
		}
		p.expect(RPAREN)
	}
	p.accept(IS)
	var decls []Decl
	for !p.at(BEGIN) && !p.at(END) && !p.at(EOF) {
		start := p.i
		if d := p.parseDecl(); d != nil {
			decls = append(decls, d)
		}
		p.ensureProgress(start, "process declaration")
	}
	p.expect(BEGIN)
	var stmts []Stmt
	for !p.at(END) && !p.at(EOF) {
		start := p.i
		if s := p.parseSequentialStmt(); s != nil {
			stmts = append(stmts, s)
		}
		p.ensureProgress(start, "sequential statement")
	}
	p.expect(END)
	p.accept(POSTPONED)
	p.expect(PROCESS)
	if p.at(IDENT) {
		p.advance() // optional closing label
	}
	p.expect(SEMICOLON)
	return &ProcessStmt{P: pos, Label: label, Postponed: postponed, Sensitivity: sens, Decls: decls, Stmts: stmts}
}

// parseSequentialStmt parses one sequential statement (assignment, procedure
// call, if/case/loop, wait, assert/report, return, next/exit, null). No
// sequential-statement keyword is deferred.
func (p *parser) parseSequentialStmt() Stmt {
	pos := p.cur().Pos
	label := ""
	if p.at(IDENT) && p.peekKind(1) == COLON {
		label = p.advance().Lit
		p.advance() // consume COLON
	}
	switch p.cur().Kind {
	case NULL:
		p.advance()
		p.expect(SEMICOLON)
		return &NullStmt{P: pos, Label: label}
	case IF:
		return p.parseIfStmt(pos, label)
	case CASE:
		return p.parseCaseStmt(pos, label)
	case FOR, WHILE, LOOP:
		return p.parseLoopStmt(pos, label)
	case NEXT:
		return p.parseNextExit(pos, label, true)
	case EXIT:
		return p.parseNextExit(pos, label, false)
	case RETURN:
		p.advance() // consume RETURN
		var val Expr
		if !p.at(SEMICOLON) {
			val = p.parseExpr()
		}
		p.expect(SEMICOLON)
		return &ReturnStmt{P: pos, Label: label, Value: val}
	case WAIT:
		return p.parseWaitStmt(pos, label)
	case ASSERT:
		return p.parseAssertStmt(pos, label)
	case REPORT:
		return p.parseReportStmt(pos, label)
	}
	target := p.parseName()
	if p.at(LE) {
		p.advance() // '<='
		delay := p.parseDelayMechanism()
		wf := p.parseWaveform()
		p.expect(SEMICOLON)
		return &SignalAssignStmt{P: pos, Label: label, Target: target, Delay: delay, Waveform: wf}
	}
	if p.at(ASSIGN) {
		p.advance() // ':='
		val := p.parseExpr()
		p.expect(SEMICOLON)
		return &VariableAssignStmt{P: pos, Label: label, Target: target, Value: val}
	}
	// procedure-call statement: name[(positional args)] ;
	switch target.(type) {
	case *CallExpr:
		p.expect(SEMICOLON)
		return p.procCall(pos, label, target)
	case *Ident:
		if p.at(SEMICOLON) {
			p.advance()
			return p.procCall(pos, label, target)
		}
	}
	p.errorf(p.cur().Pos, "deferred: sequential statement not yet parsed")
	return nil
}

// parseLoopStmt parses a for/while/bare loop:
//   [label:] for id in range loop <stmts> end loop [label] ;
//   [label:] while cond loop      <stmts> end loop [label] ;
//   [label:] loop                 <stmts> end loop [label] ;
func (p *parser) parseLoopStmt(pos Pos, label string) Stmt {
	var scheme Kind
	var param string
	var rng, cond Expr
	switch p.cur().Kind {
	case FOR:
		p.advance()
		param = p.expect(IDENT).Lit
		p.expect(IN)
		rng = p.parseDiscreteRange()
		scheme = FOR
	case WHILE:
		p.advance()
		cond = p.parseExpr()
		scheme = WHILE
	}
	// bare loop: current token is LOOP, scheme stays 0
	p.expect(LOOP)
	body := p.parseSeqStmtsUntil(END)
	p.expect(END)
	p.expect(LOOP)
	if p.at(IDENT) {
		p.advance() // optional closing label
	}
	p.expect(SEMICOLON)
	return &LoopStmt{P: pos, Label: label, Scheme: scheme, Param: param, Range: rng, Cond: cond, Stmts: body}
}

// parseNextExit parses a next/exit statement: `next|exit [loop_label] [when cond] ;`.
func (p *parser) parseNextExit(pos Pos, label string, isNext bool) Stmt {
	if isNext {
		p.expect(NEXT)
	} else {
		p.expect(EXIT)
	}
	loopLabel := ""
	if p.at(IDENT) {
		loopLabel = p.advance().Lit
	}
	var when Expr
	if p.accept(WHEN) {
		when = p.parseExpr()
	}
	p.expect(SEMICOLON)
	if isNext {
		return &NextStmt{P: pos, Label: label, LoopLabel: loopLabel, When: when}
	}
	return &ExitStmt{P: pos, Label: label, LoopLabel: loopLabel, When: when}
}

// parseWaitStmt parses `wait [on name {, name}] [until cond] [for time] ;`.
func (p *parser) parseWaitStmt(pos Pos, label string) Stmt {
	p.expect(WAIT)
	var on []Expr
	if p.accept(ON) {
		on = append(on, p.parseName())
		for p.accept(COMMA) {
			on = append(on, p.parseName())
		}
	}
	var until, forExpr Expr
	if p.accept(UNTIL) {
		until = p.parseExpr()
	}
	if p.accept(FOR) {
		forExpr = p.parseExpr()
	}
	p.expect(SEMICOLON)
	return &WaitStmt{P: pos, Label: label, On: on, Until: until, For: forExpr}
}

// parseAssertStmt parses `assert cond [report expr] [severity expr] ;`.
func (p *parser) parseAssertStmt(pos Pos, label string) Stmt {
	p.expect(ASSERT)
	cond := p.parseExpr()
	var report, severity Expr
	if p.accept(REPORT) {
		report = p.parseExpr()
	}
	if p.accept(SEVERITY) {
		severity = p.parseExpr()
	}
	p.expect(SEMICOLON)
	return &AssertStmt{P: pos, Label: label, Cond: cond, Report: report, Severity: severity}
}

// parseReportStmt parses `report expr [severity expr] ;`.
func (p *parser) parseReportStmt(pos Pos, label string) Stmt {
	p.expect(REPORT)
	report := p.parseExpr()
	var severity Expr
	if p.accept(SEVERITY) {
		severity = p.parseExpr()
	}
	p.expect(SEMICOLON)
	return &ReportStmt{P: pos, Label: label, Report: report, Severity: severity}
}

// parseSeqStmtsUntil parses sequential statements until the current token is one
// of stops (or EOF). Each iteration is ensureProgress-guarded so it cannot spin.
func (p *parser) parseSeqStmtsUntil(stops ...Kind) []Stmt {
	var stmts []Stmt
	for !p.at(EOF) && !p.atAny(stops) {
		start := p.i
		if s := p.parseSequentialStmt(); s != nil {
			stmts = append(stmts, s)
		}
		p.ensureProgress(start, "sequential statement")
	}
	return stmts
}

// atAny reports whether the current token's kind is in ks.
func (p *parser) atAny(ks []Kind) bool {
	k := p.cur().Kind
	for _, x := range ks {
		if k == x {
			return true
		}
	}
	return false
}

// parseCaseStmt parses `case expr is { when choices => <stmts> } end case [label] ;`.
func (p *parser) parseCaseStmt(pos Pos, label string) Stmt {
	p.expect(CASE)
	expr := p.parseExpr()
	p.expect(IS)
	var alts []*CaseAlt
	for p.at(WHEN) {
		p.advance() // consume WHEN
		choices := p.parseChoices()
		p.expect(ARROW)
		body := p.parseSeqStmtsUntil(WHEN, END)
		alts = append(alts, &CaseAlt{Choices: choices, Stmts: body})
	}
	p.expect(END)
	p.expect(CASE)
	if p.at(IDENT) {
		p.advance() // optional closing label
	}
	p.expect(SEMICOLON)
	return &CaseStmt{P: pos, Label: label, Expr: expr, Alts: alts}
}

// parseChoices parses `choice {| choice}` (each choice is an expression; `others`
// is an Ident, a discrete range is a Range).
func (p *parser) parseChoices() []Expr {
	var cs []Expr
	cs = append(cs, p.parseExpr())
	for p.accept(BAR) { // each iteration consumes BAR -> always advances
		cs = append(cs, p.parseExpr())
	}
	return cs
}

// parseIfStmt parses `if cond then <stmts> {elsif cond then <stmts>} [else <stmts>] end if [label] ;`.
func (p *parser) parseIfStmt(pos Pos, label string) Stmt {
	p.expect(IF)
	cond := p.parseExpr()
	p.expect(THEN)
	then := p.parseSeqStmtsUntil(ELSIF, ELSE, END)
	var elsifs []*ElsifClause
	for p.at(ELSIF) {
		p.advance()
		c := p.parseExpr()
		p.expect(THEN)
		body := p.parseSeqStmtsUntil(ELSIF, ELSE, END)
		elsifs = append(elsifs, &ElsifClause{Cond: c, Stmts: body})
	}
	var els []Stmt
	if p.accept(ELSE) {
		els = p.parseSeqStmtsUntil(END)
	}
	p.expect(END)
	p.expect(IF)
	if p.at(IDENT) {
		p.advance() // optional closing label
	}
	p.expect(SEMICOLON)
	return &IfStmt{P: pos, Label: label, Cond: cond, Then: then, Elsifs: elsifs, Else: els}
}

// isDeclStart reports whether k begins a declaration handled by parseDecl.
func isDeclStart(k Kind) bool {
	switch k {
	case CONSTANT, SIGNAL, VARIABLE, SHARED, SUBTYPE, TYPE, COMPONENT, FUNCTION, PROCEDURE, PURE, IMPURE, ATTRIBUTE, ALIAS, GROUP, FILE, USE:
		return true
	}
	// Note: FOR (configuration specification) is handled by parseDecl but is
	// intentionally absent here — in a generate body `for` introduces a nested
	// generate statement, so it must not be consumed as a declaration.
	return false
}

// finishInstantiation parses the optional generic/port maps and the terminating
// semicolon of an instantiation statement.
func (p *parser) finishInstantiation(pos Pos, label string, kind Kind, unit, arch string) Stmt {
	var gmap, pmap []*AssocElement
	if p.at(GENERIC) {
		p.advance()
		p.expect(MAP)
		gmap = p.parseAssocList()
	}
	if p.at(PORT) {
		p.advance()
		p.expect(MAP)
		pmap = p.parseAssocList()
	}
	p.expect(SEMICOLON)
	return &InstantiationStmt{P: pos, Label: label, UnitKind: kind, Unit: unit, Arch: arch, GenericMap: gmap, PortMap: pmap}
}

// parseAssocList parses `( element {, element} )`.
func (p *parser) parseAssocList() []*AssocElement {
	p.expect(LPAREN)
	var elems []*AssocElement
	if !p.at(RPAREN) {
		elems = append(elems, p.parseAssocElement())
		for p.accept(COMMA) { // each iteration consumes the comma -> always advances
			elems = append(elems, p.parseAssocElement())
		}
	}
	p.expect(RPAREN)
	return elems
}

// parseAssocElement parses `[formal =>] actual`. It parses an expression first;
// if `=>` follows, that expression was the formal (rendered to canonical text).
func (p *parser) parseAssocElement() *AssocElement {
	pos := p.cur().Pos
	first := p.parseExpr()
	if p.at(ARROW) {
		p.advance() // consume '=>'
		return &AssocElement{P: pos, Formal: exprString(first), Actual: p.parseExpr()}
	}
	return &AssocElement{P: pos, Actual: first} // positional
}

// parseDecl dispatches to the appropriate declaration parser.
func (p *parser) parseDecl() Decl {
	tok := p.cur()
	switch tok.Kind {
	case CONSTANT:
		return p.parseConstantOrSignal(true)
	case SIGNAL:
		return p.parseConstantOrSignal(false)
	case VARIABLE:
		return p.parseVariableDecl()
	case SHARED:
		return p.parseSharedVariableDecl()
	case SUBTYPE:
		return p.parseSubtypeDecl()
	case TYPE:
		return p.parseTypeDecl()
	case COMPONENT:
		return p.parseComponentDecl()
	case FUNCTION, PROCEDURE, PURE, IMPURE:
		return p.parseSubprogramDecl()
	case ATTRIBUTE:
		return p.parseAttribute()
	case ALIAS:
		return p.parseAliasDecl()
	case GROUP:
		return p.parseGroupDecl()
	case FILE:
		return p.parseFileDecl()
	case USE:
		return p.parseUseClause()
	case FOR:
		return p.parseConfigSpec()
	default:
		p.errorf(tok.Pos, "unexpected token %v %q in declaration", tok.Kind, tok.Lit)
		p.advance() // avoid infinite loop
		return nil
	}
}

// parseFileDecl parses `file names : subtype_mark [ [open expr] is expr ] ;`.
func (p *parser) parseFileDecl() Decl {
	pos := p.expect(FILE).Pos
	names := p.parseNameList()
	p.expect(COLON)
	mark := p.parseDottedName()
	var mode string
	var openMode, logical Expr
	if p.accept(OPEN) {
		openMode = p.parseExpr()
		p.expect(IS)
		logical = p.parseExpr()
	} else if p.accept(IS) {
		switch p.cur().Kind {
		case IN:
			mode = "in"
			p.advance()
		case OUT:
			mode = "out"
			p.advance()
		}
		logical = p.parseExpr()
	}
	p.expect(SEMICOLON)
	return &FileDecl{P: pos, Names: names, SubtypeMark: mark, Mode: mode, OpenMode: openMode, LogicalName: logical}
}

// parseConfigSpec parses a configuration specification:
// `for inst_list : comp binding_indication ;` (binding consumes the trailing ;).
func (p *parser) parseConfigSpec() Decl {
	pos := p.expect(FOR).Pos
	insts := []string{p.parseConfigSpecName()}
	for p.accept(COMMA) {
		insts = append(insts, p.parseConfigSpecName())
	}
	p.expect(COLON)
	comp := p.parseDottedName()
	binding := p.parseBindingIndication()
	return &ConfigSpec{P: pos, Insts: insts, Comp: comp, Binding: binding}
}

// parseSubprogramDecl parses a subprogram SPECIFICATION:
//
//	[pure|impure] function designator [(params)] return mark ;
//	procedure designator [(params)] ;
//
// When `is` follows the spec, it parses a subprogram body (declarations,
// `begin`, sequential statements, `end`) and returns a *SubprogramBody;
// otherwise it returns a spec-only *SubprogramDecl.
func (p *parser) parseSubprogramDecl() Decl {
	pos := p.cur().Pos
	var pure, impure bool
	if p.accept(PURE) {
		pure = true
	} else if p.accept(IMPURE) {
		impure = true
	}
	isProc := false
	if p.accept(PROCEDURE) {
		isProc = true
	} else {
		p.expect(FUNCTION)
	}
	// designator: identifier or operator-symbol string literal
	var desig string
	switch p.cur().Kind {
	case IDENT, EXTIDENT, STRINGLIT:
		desig = p.advance().Lit
	default:
		t := p.cur()
		p.errorf(t.Pos, "expected subprogram designator, got %v %q", t.Kind, t.Lit)
		p.advance() // consume the bad token so downstream steps do not re-error on it
	}
	// Params reuse parseInterfaceList (name : [mode] subtype). Object-class
	// prefixes (constant/signal/variable) on a parameter are not handled; such
	// a subprogram would error and its file be excluded. Not seen in the corpus.
	var params []*InterfaceDecl
	if p.at(LPAREN) {
		params = p.parseInterfaceList()
	}
	var ret string
	if !isProc {
		p.expect(RETURN)
		ret = p.parseDottedName()
	}
	d := &SubprogramDecl{P: pos, IsProcedure: isProc, Pure: pure, Impure: impure, Designator: desig, Params: params, ReturnMark: ret}
	if p.at(IS) {
		p.advance() // consume IS
		var decls []Decl
		for !p.at(BEGIN) && !p.at(END) && !p.at(EOF) {
			start := p.i
			if dd := p.parseDecl(); dd != nil {
				decls = append(decls, dd)
			}
			p.ensureProgress(start, "subprogram declaration")
		}
		p.expect(BEGIN)
		stmts := p.parseSeqStmtsUntil(END)
		p.expect(END)
		p.accept(FUNCTION)  // optional kind keyword in `end function`/`end procedure`
		p.accept(PROCEDURE)
		if p.at(IDENT) || p.at(EXTIDENT) || p.at(STRINGLIT) {
			p.advance() // optional closing designator
		}
		p.expect(SEMICOLON)
		return &SubprogramBody{P: pos, IsProcedure: isProc, Pure: pure, Impure: impure, Designator: desig, Params: params, ReturnMark: ret, Decls: decls, Stmts: stmts}
	}
	p.expect(SEMICOLON)
	return d
}

// parseConstantOrSignal parses CONSTANT or SIGNAL declarations.
func (p *parser) parseConstantOrSignal(isConst bool) Decl {
	pos := p.advance().Pos // consume CONSTANT or SIGNAL
	names := p.parseNameList()
	p.expect(COLON)
	mark, constraint := p.parseSubtypeIndication()
	var def Expr
	if p.accept(ASSIGN) {
		def = p.parseExpr()
	}
	p.expect(SEMICOLON)
	if isConst {
		return &ConstantDecl{P: pos, Names: names, SubtypeMark: mark, Constraint: constraint, Default: def}
	}
	return &SignalDecl{P: pos, Names: names, SubtypeMark: mark, Constraint: constraint, Default: def}
}

// parseVariableDecl parses `variable names : subtype [:= default] ;`.
func (p *parser) parseVariableDecl() Decl {
	pos := p.expect(VARIABLE).Pos
	return p.finishVariableDecl(pos, false)
}

// parseSharedVariableDecl parses `shared variable … ;`.
func (p *parser) parseSharedVariableDecl() Decl {
	pos := p.expect(SHARED).Pos
	p.expect(VARIABLE)
	return p.finishVariableDecl(pos, true)
}

// finishVariableDecl parses the body after the [shared] variable keyword(s):
// `names : subtype [:= default] ;`.
func (p *parser) finishVariableDecl(pos Pos, shared bool) Decl {
	names := p.parseNameList()
	p.expect(COLON)
	mark, constraint := p.parseSubtypeIndication()
	var def Expr
	if p.accept(ASSIGN) {
		def = p.parseExpr()
	}
	p.expect(SEMICOLON)
	return &VariableDecl{P: pos, Shared: shared, Names: names, SubtypeMark: mark, Constraint: constraint, Default: def}
}

// parseSubtypeDecl parses: SUBTYPE id IS subtype-indication ;
func (p *parser) parseSubtypeDecl() *SubtypeDecl {
	pos := p.expect(SUBTYPE).Pos
	name := p.expect(IDENT).Lit
	p.expect(IS)
	mark, constraint := p.parseSubtypeIndication()
	p.expect(SEMICOLON)
	return &SubtypeDecl{P: pos, Name: name, SubtypeMark: mark, Constraint: constraint}
}

// parseTypeDecl parses: TYPE id IS type-definition ;
func (p *parser) parseTypeDecl() *TypeDecl {
	pos := p.expect(TYPE).Pos
	name := p.expect(IDENT).Lit
	p.expect(IS)

	var def TypeDef
	switch p.cur().Kind {
	case LPAREN:
		// Enumeration type: ( lit1, lit2, ... )
		epos := p.cur().Pos
		p.advance() // consume '('
		var lits []string
		lits = append(lits, p.parseEnumLit())
		for p.accept(COMMA) {
			lits = append(lits, p.parseEnumLit())
		}
		p.expect(RPAREN)
		def = &EnumDef{P: epos, Lits: lits}

	case RECORD:
		rpos := p.advance().Pos // consume RECORD
		var fields []RecordField
		for !p.at(END) && !p.at(EOF) {
			start := p.i
			names := p.parseNameList()
			p.expect(COLON)
			mark, constraint := p.parseSubtypeIndication()
			p.expect(SEMICOLON)
			fields = append(fields, RecordField{Names: names, SubtypeMark: mark, Constraint: constraint})
			p.ensureProgress(start, "record field")
		}
		p.expect(END)
		p.accept(RECORD)
		// optional type name
		if p.at(IDENT) {
			p.advance()
		}
		def = &RecordDef{P: rpos, Fields: fields}

	case ARRAY:
		apos := p.cur().Pos
		// Capture everything up to the semicolon as raw text
		var parts []string
		for !p.at(SEMICOLON) && !p.at(EOF) {
			tok := p.advance()
			text := tok.Lit
			if text == "" {
				text = tok.Kind.String()
			}
			parts = append(parts, text)
		}
		def = &ArrayDef{P: apos, Text: strings.Join(parts, " ")}

	case FILE:
		fpos := p.advance().Pos // consume FILE
		p.expect(OF)
		def = &FileTypeDef{P: fpos, Mark: p.parseDottedName()}

	case ACCESS:
		apos := p.advance().Pos // consume ACCESS
		mark, _ := p.parseSubtypeIndication()
		def = &AccessDef{P: apos, Mark: mark}

	default:
		p.errorf(p.cur().Pos, "unsupported type definition starting with %v", p.cur().Kind)
		// consume until semicolon
		for !p.at(SEMICOLON) && !p.at(EOF) {
			p.advance()
		}
	}
	p.expect(SEMICOLON)
	return &TypeDecl{P: pos, Name: name, Def: def}
}

// parseEnumLit parses an enumeration literal: identifier or character literal.
func (p *parser) parseEnumLit() string {
	tok := p.cur()
	if tok.Kind == IDENT || tok.Kind == CHARLIT {
		p.advance()
		return tok.Lit
	}
	p.errorf(tok.Pos, "expected enum literal, got %v %q", tok.Kind, tok.Lit)
	p.advance()
	return ""
}

// parseComponentDecl parses: COMPONENT id [IS] [GENERIC(...);] [PORT(...);] END COMPONENT [id] ;
func (p *parser) parseComponentDecl() *ComponentDecl {
	pos := p.expect(COMPONENT).Pos
	name := p.expect(IDENT).Lit
	p.accept(IS)

	var generics, ports []*InterfaceDecl
	if p.at(GENERIC) {
		p.advance() // consume GENERIC
		generics = p.parseInterfaceList()
		p.expect(SEMICOLON)
	}
	if p.at(PORT) {
		p.advance() // consume PORT
		ports = p.parseInterfaceList()
		p.expect(SEMICOLON)
	}

	p.expect(END)
	p.accept(COMPONENT)
	if p.at(IDENT) {
		p.advance()
	}
	p.expect(SEMICOLON)
	return &ComponentDecl{P: pos, Name: name, Generics: generics, Ports: ports}
}

// parseInterfaceList parses: ( iface {; iface} )
func (p *parser) parseInterfaceList() []*InterfaceDecl {
	p.expect(LPAREN)
	var decls []*InterfaceDecl
	if !p.at(RPAREN) {
		decls = append(decls, p.parseInterfaceDecl())
		for p.accept(SEMICOLON) {
			if p.at(RPAREN) {
				break
			}
			decls = append(decls, p.parseInterfaceDecl())
		}
	}
	p.expect(RPAREN)
	return decls
}

// parseInterfaceDecl parses: name-list : [mode] subtype-indication [:= expr]
func (p *parser) parseInterfaceDecl() *InterfaceDecl {
	pos := p.cur().Pos
	// Optional object-class prefix on a (subprogram) interface element:
	// constant/signal/variable. Captured so round-trip is faithful.
	objClass := ""
	switch p.cur().Kind {
	case CONSTANT:
		objClass = "constant"
		p.advance()
	case SIGNAL:
		objClass = "signal"
		p.advance()
	case VARIABLE:
		objClass = "variable"
		p.advance()
	}
	names := p.parseNameList()
	p.expect(COLON)

	// Optional mode keyword
	mode := ""
	switch p.cur().Kind {
	case IN:
		mode = "in"
		p.advance()
	case OUT:
		mode = "out"
		p.advance()
	case INOUT:
		mode = "inout"
		p.advance()
	case BUFFER:
		mode = "buffer"
		p.advance()
	case LINKAGE:
		mode = "linkage"
		p.advance()
	}

	mark, constraint := p.parseSubtypeIndication()
	var def Expr
	if p.accept(ASSIGN) {
		def = p.parseExpr()
	}
	return &InterfaceDecl{P: pos, ObjClass: objClass, Names: names, Mode: mode, SubtypeMark: mark, Constraint: constraint, Default: def}
}

// parseNameList parses: id {, id}
func (p *parser) parseNameList() []string {
	var names []string
	names = append(names, p.expect(IDENT).Lit)
	for p.accept(COMMA) {
		names = append(names, p.expect(IDENT).Lit)
	}
	return names
}

// parseSubtypeIndication parses a subtype indication: type-mark [(constraint)]
// Returns the type mark as a string and an optional constraint expression.
func (p *parser) parseSubtypeIndication() (mark string, constraint Expr) {
	// Parse a (possibly dotted) type mark
	mark = p.parseDottedName()

	// Optional constraint
	switch p.cur().Kind {
	case LPAREN:
		// Index or range constraint — parse as an expression
		constraint = p.parseParenOrAggregate()
	case RANGE:
		p.advance() // consume RANGE keyword
		constraint = p.parseExpr()
	}
	return mark, constraint
}

// parseAttribute parses an attribute declaration (`attribute n : mark ;`) or an
// attribute specification (`attribute n of entities : class is expr ;`).
func (p *parser) parseAttribute() Decl {
	pos := p.expect(ATTRIBUTE).Pos
	name := p.expect(IDENT).Lit
	if p.accept(OF) {
		entities := []string{p.parseEntityName()}
		for p.accept(COMMA) {
			entities = append(entities, p.parseEntityName())
		}
		p.expect(COLON)
		classTok := p.advance()
		class := classTok.Kind
		if class <= kwStart || class >= kwEnd {
			// entity_class must be a reserved word (signal/subtype/variable/...);
			// reject non-keywords so malformed specs are excluded, not mis-parsed.
			p.errorf(classTok.Pos, "expected entity class keyword, got %v %q", class, classTok.Lit)
		}
		p.expect(IS)
		val := p.parseExpr()
		p.expect(SEMICOLON)
		return &AttributeSpec{P: pos, Name: name, Entities: entities, EntityClass: class, Value: val}
	}
	p.expect(COLON)
	mark := p.parseDottedName()
	p.expect(SEMICOLON)
	return &AttributeDecl{P: pos, Name: name, TypeMark: mark}
}

// parseEntityName reads one entity name in an attribute spec's name list: an
// identifier, or the keywords `others`/`all`.
func (p *parser) parseEntityName() string {
	switch p.cur().Kind {
	case IDENT:
		return p.advance().Lit
	case OTHERS, ALL:
		return p.advance().Kind.String()
	default:
		t := p.cur()
		p.errorf(t.Pos, "expected entity name, got %v %q", t.Kind, t.Lit)
		p.advance() // ensure progress
		return ""
	}
}

// parseSignature parses `[ [type_mark {, type_mark}] [return type_mark] ]`.
func (p *parser) parseSignature() *Signature {
	p.expect(LBRACKET)
	sig := &Signature{}
	if !p.at(RETURN) && !p.at(RBRACKET) {
		sig.Types = append(sig.Types, p.parseDottedName())
		for p.accept(COMMA) {
			sig.Types = append(sig.Types, p.parseDottedName())
		}
	}
	if p.accept(RETURN) {
		sig.Return = p.parseDottedName()
	}
	p.expect(RBRACKET)
	return sig
}

// parseAliasDecl parses `alias name [: subtype_indication] is target [signature] ;`.
func (p *parser) parseAliasDecl() Decl {
	pos := p.expect(ALIAS).Pos
	name := p.expect(IDENT).Lit
	var mark string
	var constraint Expr
	if p.accept(COLON) {
		mark, constraint = p.parseSubtypeIndication()
	}
	p.expect(IS)
	target := p.parseName()
	var sig *Signature
	if p.at(LBRACKET) {
		sig = p.parseSignature()
	}
	p.expect(SEMICOLON)
	return &AliasDecl{P: pos, Name: name, SubtypeMark: mark, Constraint: constraint, Target: target, Signature: sig}
}

// parseGroupDecl parses a group template declaration (`group n is (classes) ;`)
// or a group declaration (`group n : template (constituents) ;`).
func (p *parser) parseGroupDecl() Decl {
	pos := p.expect(GROUP).Pos
	name := p.expect(IDENT).Lit
	if p.accept(IS) {
		// template: ( entity_class [<>] {, ...} )
		p.expect(LPAREN)
		var classes []string
		classes = append(classes, p.parseEntityClassEntry())
		for p.accept(COMMA) {
			classes = append(classes, p.parseEntityClassEntry())
		}
		p.expect(RPAREN)
		p.expect(SEMICOLON)
		return &GroupTemplateDecl{P: pos, Name: name, Classes: classes}
	}
	p.expect(COLON)
	tmpl := p.parseDottedName()
	p.expect(LPAREN)
	var cons []string
	cons = append(cons, p.parseDottedName())
	for p.accept(COMMA) {
		cons = append(cons, p.parseDottedName())
	}
	p.expect(RPAREN)
	p.expect(SEMICOLON)
	return &GroupDecl{P: pos, Name: name, TemplateMark: tmpl, Constituents: cons}
}

// parseEntityClassEntry reads one `entity_class [<>]` in a group template.
func (p *parser) parseEntityClassEntry() string {
	t := p.advance() // entity-class keyword
	s := t.Kind.String() // canonical lowercase; ignore raw source casing
	if p.accept(BOX) {
		s += " <>"
	}
	return s
}

// isAttrNameKind reports whether k can name an attribute after a tick
// (an identifier, extended identifier, or a reserved word like `range`).
func isAttrNameKind(k Kind) bool {
	return k == IDENT || k == EXTIDENT || (k > kwStart && k < kwEnd)
}

// parseName parses an identifier (possibly dotted or with attribute ticks)
// and optionally a call-or-index suffix.
func (p *parser) parseName() Expr {
	tok := p.advance() // IDENT or EXTIDENT
	pos := tok.Pos
	text := tok.Lit

	// Dotted name: consume . id sequences.
	for p.at(DOT) {
		p.advance() // consume '.'
		next := p.cur()
		if next.Kind == IDENT || next.Kind == EXTIDENT || next.Kind == ALL {
			p.advance()
			seg := next.Lit
			if seg == "" {
				seg = next.Kind.String()
			}
			text += "." + seg
		} else {
			// Not a valid continuation — stop here.
			break
		}
	}

	// Attribute tick: 'attrname sequences.  TICK must be followed by an
	// identifier or keyword to be an attribute (not a character literal).
	for p.at(TICK) {
		// Peek at the token after the tick. peekKind returns EOF past the end,
		// and EOF is not an attr-name kind, so the loop still breaks correctly.
		ak := p.peekKind(1)
		if !isAttrNameKind(ak) {
			break
		}
		p.advance() // consume TICK
		attr := p.advance()
		attrText := attr.Lit
		if attrText == "" {
			attrText = attr.Kind.String()
		}
		text += "'" + attrText
	}

	name := &Ident{NamePos: pos, Name: text}

	// Qualified expression: mark'(...). The attribute loop above already
	// consumed any 'attr; a remaining tick directly before '(' is qualification,
	// not an attribute or a character literal.
	if p.at(TICK) && p.peekKind(1) == LPAREN {
		tick := p.advance() // consume '
		return &QualifiedExpr{Mark: name, Tick: tick.Pos, X: p.parseParenOrAggregate()}
	}

	// Suffix chain: zero or more `(args)` (call/index/slice), `.field`
	// (selection after a call/index), and `'attr` (attribute on a call/index
	// result). A leading run of simple `.id` is already flattened into the Ident
	// above; this loop builds chained CallExpr, SelectorExpr, and AttributeName
	// for the non-flat tail.
	var expr Expr = name
	for {
		switch {
		case p.at(LPAREN):
			lparen := p.advance() // consume '('
			var args []*AssocElement
			if !p.at(RPAREN) {
				args = append(args, p.parseAssocElement())
				for p.accept(COMMA) {
					args = append(args, p.parseAssocElement())
				}
			}
			rparen := p.expect(RPAREN)
			expr = &CallExpr{Fun: expr, Lparen: lparen.Pos, Args: args, Rparen: rparen.Pos}
		case p.at(DOT) && (p.peekKind(1) == IDENT || p.peekKind(1) == EXTIDENT || p.peekKind(1) == ALL):
			dotTok := p.advance() // consume '.'
			selTok := p.advance()
			sel := selTok.Lit
			if sel == "" {
				sel = selTok.Kind.String()
			}
			expr = &SelectorExpr{X: expr, Dot: dotTok.Pos, Sel: sel}
		case p.at(TICK) && isAttrNameKind(p.peekKind(1)):
			tickTok := p.advance() // consume '
			attr := p.advance()
			attrText := attr.Lit
			if attrText == "" {
				attrText = attr.Kind.String()
			}
			expr = &AttributeName{X: expr, Tick: tickTok.Pos, Attr: attrText}
		default:
			return expr
		}
	}
}
