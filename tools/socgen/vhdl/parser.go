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
}

// newParser lexes all tokens from src, dropping COMMENT tokens, and returns a
// parser ready to consume the stream. The final EOF token is retained.
func newParser(src []byte) *parser {
	l := NewLexer(src, "")
	var toks []Token
	for {
		tok := l.Next()
		if tok.Kind == COMMENT {
			continue
		}
		toks = append(toks, tok)
		if tok.Kind == EOF {
			break
		}
	}
	return &parser{toks: toks}
}

// newParserFromExpr is a test seam — identical to newParser for P1a.
func newParserFromExpr(src []byte) *parser {
	return newParser(src)
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
	p.errs = append(p.errs, fmt.Errorf("%v: expected %v, got %v %q", tok.Pos, k, tok.Kind, tok.Lit))
	return tok
}

// isBinaryOp reports whether kind k is one of the P1a binary operator tokens.
func isBinaryOp(k Kind) bool {
	switch k {
	case PLUS, MINUS, STAR, SLASH, AMP,
		EQ, NE, LT, LE, GT, GE,
		AND, OR, NAND, NOR, XOR, XNOR,
		MOD, REM, SLL, SRL, SLA, SRA, ROL, ROR:
		return true
	}
	return false
}

// opString returns the canonical text for a binary operator token.
func opString(tok Token) string {
	if tok.Lit != "" {
		return tok.Lit
	}
	return tok.Kind.String()
}

// parseExpr parses a P1a expression (single-precedence binary ops + optional
// range direction suffix).
func (p *parser) parseExpr() Expr {
	left := p.parsePrimary()
	pos := left.Pos()

	// Left-fold binary operators.
	for isBinaryOp(p.cur().Kind) {
		op := p.advance()
		right := p.parsePrimary()
		left = &BinaryExpr{P: pos, Op: opString(op), X: left, Y: right}
	}

	// Optional range suffix.
	if p.at(TO) || p.at(DOWNTO) {
		dir := p.advance()
		right := p.parsePrimary()
		// Also fold any binary ops on the right side.
		for isBinaryOp(p.cur().Kind) {
			op := p.advance()
			rr := p.parsePrimary()
			right = &BinaryExpr{P: right.Pos(), Op: opString(op), X: right, Y: rr}
		}
		dirStr := "to"
		if dir.Kind == DOWNTO {
			dirStr = "downto"
		}
		return &Range{P: pos, Left: left, Dir: dirStr, Right: right}
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
		return &Lit{P: tok.Pos, Text: tok.Lit}

	case LPAREN:
		return p.parseParen()

	case IDENT, EXTIDENT:
		return p.parseName()

	default:
		// Keywords that can appear as primaries in expressions (e.g. inside
		// aggregates: others, all; attribute names: range, etc.)
		switch tok.Kind {
		case OTHERS, ALL, RANGE, NULL, OPEN:
			p.advance()
			return &Name{P: tok.Pos, Text: tok.Kind.String()}
		}
		// Unrecognised primary — record an error and return an empty literal so
		// the caller can continue.
		p.errs = append(p.errs, fmt.Errorf("%v: unexpected token %v %q in primary", tok.Pos, tok.Kind, tok.Lit))
		return &Lit{P: tok.Pos, Text: ""}
	}
}

// parseParen parses a parenthesized expression.  For P1a the interior is
// captured verbatim as a Lit (with tokens joined by spaces) and wrapped in a
// Paren node.  This keeps the round-trip stable without needing full aggregate
// parsing, which is deferred to P1b.
func (p *parser) parseParen() Expr {
	open := p.expect(LPAREN)
	pos := open.Pos

	// Collect inner tokens at any nesting depth, stopping at the matching ')'.
	depth := 1
	var parts []string
	for depth > 0 && !p.at(EOF) {
		tok := p.advance()
		switch tok.Kind {
		case LPAREN:
			depth++
		case RPAREN:
			depth--
			if depth == 0 {
				// This was the closing paren — don't include it.
				goto done
			}
		}
		// Represent each token by its literal text if available, else its kind string.
		text := tok.Lit
		if text == "" {
			text = tok.Kind.String()
		}
		parts = append(parts, text)
	}
done:
	inner := strings.Join(parts, " ")
	return &Paren{P: pos, X: &Lit{P: pos, Text: inner}}
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
		next2 := p.toks[p.i+0] // that's the TICK itself — check what follows
		_ = next2
		// Peek at token after the tick.
		if p.i+1 >= len(p.toks) {
			break
		}
		after := p.toks[p.i+1]
		isAttrName := after.Kind == IDENT || after.Kind == EXTIDENT ||
			(after.Kind > kwStart && after.Kind < kwEnd)
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

	name := &Name{P: pos, Text: text}

	// Call or index: name ( args ).
	if p.at(LPAREN) {
		p.advance() // consume '('
		var args []Expr
		if !p.at(RPAREN) {
			args = append(args, p.parseExpr())
			for p.accept(COMMA) {
				args = append(args, p.parseExpr())
			}
		}
		p.expect(RPAREN)
		return &CallOrIndex{P: pos, Prefix: name, Args: args}
	}

	return name
}
