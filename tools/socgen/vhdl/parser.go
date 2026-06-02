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
	case INT, REAL, BASEDLIT, CHARLIT, STRINGLIT, BITSTRINGLIT:
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

// parseFile parses a complete VHDL design file: optional context clauses
// followed by one or more design units.
func (p *parser) parseFile() *DesignFile {
	df := &DesignFile{}

	// Context clauses: library / use
	for p.at(LIBRARY) || p.at(USE) {
		switch p.cur().Kind {
		case LIBRARY:
			pos := p.advance().Pos // consume LIBRARY
			var names []string
			names = append(names, p.expect(IDENT).Lit)
			for p.accept(COMMA) {
				names = append(names, p.expect(IDENT).Lit)
			}
			p.expect(SEMICOLON)
			df.Context = append(df.Context, &LibraryClause{P: pos, Names: names})
		case USE:
			pos := p.advance().Pos // consume USE
			var names []string
			names = append(names, p.parseDottedName())
			for p.accept(COMMA) {
				names = append(names, p.parseDottedName())
			}
			p.expect(SEMICOLON)
			df.Context = append(df.Context, &UseClause{P: pos, Names: names})
		}
	}

	// Design units
	for !p.at(EOF) {
		start := p.i
		switch p.cur().Kind {
		case PACKAGE:
			// Peek: PACKAGE BODY → deferred
			if p.peekKind(1) == BODY {
				p.errorf(p.cur().Pos, "deferred: package body not yet parsed")
				return df
			}
			u := p.parsePackageDecl()
			if u != nil {
				df.Units = append(df.Units, u)
			}
		case ENTITY:
			u := p.parseEntityDecl()
			if u != nil {
				df.Units = append(df.Units, u)
			}
		case ARCHITECTURE:
			p.errorf(p.cur().Pos, "deferred: architecture body not yet parsed")
			return df
		case CONFIGURATION:
			p.errorf(p.cur().Pos, "deferred: configuration not yet parsed")
			return df
		default:
			p.errorf(p.cur().Pos, "unexpected token %v %q at top level", p.cur().Kind, p.cur().Lit)
			return df
		}
		p.ensureProgress(start, "design unit")
	}
	return df
}

// ParseFile parses src (named filename) into a DesignFile. fset records source
// positions; returned errors are rendered via fset.Position. A nil or empty
// error slice means a clean parse.
func ParseFile(fset *FileSet, filename string, src []byte) (*DesignFile, []error) {
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

	p.expect(END)
	p.accept(ENTITY)
	if p.at(IDENT) {
		p.advance()
	}
	p.expect(SEMICOLON)
	return &EntityDecl{P: pos, Name: name, Generics: generics, Ports: ports}
}

// parseDecl dispatches to the appropriate declaration parser.
func (p *parser) parseDecl() Decl {
	tok := p.cur()
	switch tok.Kind {
	case CONSTANT:
		return p.parseConstantOrSignal(true)
	case SIGNAL:
		return p.parseConstantOrSignal(false)
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
	default:
		p.errorf(tok.Pos, "unexpected token %v %q in declaration", tok.Kind, tok.Lit)
		p.advance() // avoid infinite loop
		return nil
	}
}

// parseSubprogramDecl parses a subprogram SPECIFICATION:
//
//	[pure|impure] function designator [(params)] return mark ;
//	procedure designator [(params)] ;
//
// A body (`... is ...`) is deferred.
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
	case IDENT, STRINGLIT:
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
		// Subprogram body: deferred in this phase. The `is` and body tokens are
		// left unconsumed; the enclosing declarative loop's ensureProgress skips
		// them (with further errors), so files containing subprogram bodies are
		// excluded from round-trip rather than parsed. TODO: parse bodies in P1d.
		p.errorf(p.cur().Pos, "deferred: subprogram body not yet parsed")
		return d
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
	return &InterfaceDecl{P: pos, Names: names, Mode: mode, SubtypeMark: mark, Constraint: constraint, Default: def}
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
		class := p.advance().Kind // entity_class keyword (signal/subtype/variable/...)
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
		isAttrName := ak == IDENT || ak == EXTIDENT || (ak > kwStart && ak < kwEnd)
		if !isAttrName {
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

	// Note: an attribute applied to a call/indexed result (e.g. f(a)'length) is
	// not handled here — the attribute loop above runs before this suffix. Such
	// names need the SelectorExpr/attribute decomposition deferred past P1b.
	// TODO(p1c): handle suffix attributes when names are properly decomposed.

	// Call or index: name ( args ).
	if p.at(LPAREN) {
		lparen := p.advance() // consume '('
		var args []Expr
		if !p.at(RPAREN) {
			args = append(args, p.parseExpr())
			for p.accept(COMMA) {
				args = append(args, p.parseExpr())
			}
		}
		rparen := p.expect(RPAREN)
		return &CallExpr{Fun: name, Lparen: lparen.Pos, Args: args, Rparen: rparen.Pos}
	}

	return name
}
