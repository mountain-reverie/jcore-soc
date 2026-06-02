package vhdl

// Lexer holds the state for lexing a single VHDL source file.
type Lexer struct {
	src  []byte
	file string
	off  int // current byte offset into src
	line int // 1-based
	col  int // 1-based
}

// NewLexer returns a new Lexer for the given source bytes.
func NewLexer(src []byte, file string) *Lexer {
	return &Lexer{src: src, file: file, line: 1, col: 1}
}

// peek returns the byte at the current offset, or 0 if at end.
func (l *Lexer) peek() byte {
	if l.off >= len(l.src) {
		return 0
	}
	return l.src[l.off]
}

// peekAt returns the byte at offset+n, or 0 if out of range.
func (l *Lexer) peekAt(n int) byte {
	i := l.off + n
	if i >= len(l.src) {
		return 0
	}
	return l.src[i]
}

// advance consumes one byte, updating line/col tracking.
func (l *Lexer) advance() byte {
	if l.off >= len(l.src) {
		return 0
	}
	c := l.src[l.off]
	l.off++
	if c == '\n' {
		l.line++
		l.col = 1
	} else {
		l.col++
	}
	return c
}

func isLetter(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func isDigit(c byte) bool {
	return c >= '0' && c <= '9'
}

func isIdentStart(c byte) bool {
	return isLetter(c)
}

func isIdentCont(c byte) bool {
	return isLetter(c) || isDigit(c) || c == '_'
}

// Next returns the next token from the source.
func (l *Lexer) Next() Token {
	// Skip whitespace.
	for l.off < len(l.src) {
		c := l.peek()
		if c == ' ' || c == '\t' || c == '\r' || c == '\n' {
			l.advance()
		} else {
			break
		}
	}

	if l.off >= len(l.src) {
		return Token{Kind: EOF, Pos: Pos{Line: l.line, Col: l.col, Offset: l.off}}
	}

	startPos := Pos{Line: l.line, Col: l.col, Offset: l.off}
	c := l.peek()

	// -- comment
	if c == '-' && l.peekAt(1) == '-' {
		start := l.off
		// consume until end of line or EOF
		for l.off < len(l.src) && l.peek() != '\n' {
			l.advance()
		}
		return Token{Kind: COMMENT, Lit: string(l.src[start:l.off]), Pos: startPos}
	}

	// Identifier or keyword
	if isIdentStart(c) {
		start := l.off
		for l.off < len(l.src) && isIdentCont(l.peek()) {
			l.advance()
		}
		lit := string(l.src[start:l.off])
		if kind, ok := LookupKeyword(lit); ok {
			return Token{Kind: kind, Lit: lit, Pos: startPos}
		}
		return Token{Kind: IDENT, Lit: lit, Pos: startPos}
	}

	// Integer literal (decimal only for this task)
	if isDigit(c) {
		start := l.off
		for l.off < len(l.src) {
			ch := l.peek()
			if isDigit(ch) || ch == '_' {
				l.advance()
			} else {
				break
			}
		}
		return Token{Kind: INT, Lit: string(l.src[start:l.off]), Pos: startPos}
	}

	// Delimiters — maximal munch.
	l.advance() // consume first char
	switch c {
	case '(':
		return Token{Kind: LPAREN, Pos: startPos}
	case ')':
		return Token{Kind: RPAREN, Pos: startPos}
	case ',':
		return Token{Kind: COMMA, Pos: startPos}
	case ';':
		return Token{Kind: SEMICOLON, Pos: startPos}
	case '+':
		return Token{Kind: PLUS, Pos: startPos}
	case '-':
		return Token{Kind: MINUS, Pos: startPos}
	case '&':
		return Token{Kind: AMP, Pos: startPos}
	case '|':
		return Token{Kind: BAR, Pos: startPos}
	case '.':
		return Token{Kind: DOT, Pos: startPos}
	case '\'':
		return Token{Kind: TICK, Pos: startPos}
	case ':':
		if l.peek() == '=' {
			l.advance()
			return Token{Kind: ASSIGN, Pos: startPos}
		}
		return Token{Kind: COLON, Pos: startPos}
	case '=':
		if l.peek() == '>' {
			l.advance()
			return Token{Kind: ARROW, Pos: startPos}
		}
		return Token{Kind: EQ, Pos: startPos}
	case '>':
		if l.peek() == '=' {
			l.advance()
			return Token{Kind: GE, Pos: startPos}
		}
		return Token{Kind: GT, Pos: startPos}
	case '<':
		if l.peek() == '=' {
			l.advance()
			return Token{Kind: LE, Pos: startPos}
		}
		if l.peek() == '>' {
			l.advance()
			return Token{Kind: BOX, Pos: startPos}
		}
		return Token{Kind: LT, Pos: startPos}
	case '/':
		if l.peek() == '=' {
			l.advance()
			return Token{Kind: NE, Pos: startPos}
		}
		return Token{Kind: SLASH, Pos: startPos}
	case '*':
		if l.peek() == '*' {
			l.advance()
			return Token{Kind: EXP, Pos: startPos}
		}
		return Token{Kind: STAR, Pos: startPos}
	default:
		return Token{Kind: ILLEGAL, Lit: string(c), Pos: startPos}
	}
}
