package vhdl

import "fmt"

// Kind is the lexical category of a token.
type Kind int

const (
	ILLEGAL Kind = iota
	EOF
	COMMENT

	// literals and names
	IDENT
	EXTIDENT
	INT
	REAL
	BASEDLIT
	CHARLIT
	STRINGLIT
	BITSTRINGLIT

	// delimiters (VHDL-93)
	LPAREN    // (
	RPAREN    // )
	COMMA     // ,
	SEMICOLON // ;
	COLON     // :
	ASSIGN    // :=
	ARROW     // =>
	LE        // <=
	GE        // >=
	NE        // /=
	LT        // <
	GT        // >
	EQ        // =
	PLUS      // +
	MINUS     // -
	STAR      // *
	SLASH     // /
	EXP       // **
	AMP       // &
	BAR       // |
	DOT       // .
	TICK      // '
	BOX       // <>

	// keywords (kept between kwStart/kwEnd sentinels so the keyword table
	// can be built by iterating the range)
	kwStart
	ABS
	ACCESS
	AFTER
	ALIAS
	ALL
	AND
	ARCHITECTURE
	ARRAY
	ASSERT
	ATTRIBUTE
	BEGIN
	BLOCK
	BODY
	BUFFER
	BUS
	CASE
	COMPONENT
	CONFIGURATION
	CONSTANT
	DISCONNECT
	DOWNTO
	ELSE
	ELSIF
	END
	ENTITY
	EXIT
	FILE
	FOR
	FUNCTION
	GENERATE
	GENERIC
	GROUP
	GUARDED
	IF
	IMPURE
	IN
	INERTIAL
	INOUT
	IS
	LABEL
	LIBRARY
	LINKAGE
	LITERAL
	LOOP
	MAP
	MOD
	NAND
	NEW
	NEXT
	NOR
	NOT
	NULL
	OF
	ON
	OPEN
	OR
	OTHERS
	OUT
	PACKAGE
	PORT
	POSTPONED
	PROCEDURE
	PROCESS
	PURE
	RANGE
	RECORD
	REGISTER
	REJECT
	REM
	REPORT
	RETURN
	ROL
	ROR
	SELECT
	SEVERITY
	SHARED
	SIGNAL
	SLA
	SLL
	SRA
	SRL
	SUBTYPE
	THEN
	TO
	TRANSPORT
	TYPE
	UNAFFECTED
	UNITS
	UNTIL
	USE
	VARIABLE
	WAIT
	WHEN
	WHILE
	WITH
	XNOR
	XOR
	kwEnd
)

// kindStr maps every named Kind to its canonical text: keyword kinds to their
// lowercase spelling, delimiter kinds to their symbol, and the rest to an
// uppercase Go-style name.
var kindStr = map[Kind]string{
	ILLEGAL: "ILLEGAL", EOF: "EOF", COMMENT: "COMMENT",
	IDENT: "IDENT", EXTIDENT: "EXTIDENT", INT: "INT", REAL: "REAL",
	BASEDLIT: "BASEDLIT", CHARLIT: "CHARLIT", STRINGLIT: "STRINGLIT", BITSTRINGLIT: "BITSTRINGLIT",

	LPAREN: "(", RPAREN: ")", COMMA: ",", SEMICOLON: ";", COLON: ":",
	ASSIGN: ":=", ARROW: "=>", LE: "<=", GE: ">=", NE: "/=", LT: "<", GT: ">",
	EQ: "=", PLUS: "+", MINUS: "-", STAR: "*", SLASH: "/", EXP: "**",
	AMP: "&", BAR: "|", DOT: ".", TICK: "'", BOX: "<>",

	ABS: "abs", ACCESS: "access", AFTER: "after", ALIAS: "alias", ALL: "all",
	AND: "and", ARCHITECTURE: "architecture", ARRAY: "array", ASSERT: "assert",
	ATTRIBUTE: "attribute", BEGIN: "begin", BLOCK: "block", BODY: "body",
	BUFFER: "buffer", BUS: "bus", CASE: "case", COMPONENT: "component",
	CONFIGURATION: "configuration", CONSTANT: "constant", DISCONNECT: "disconnect",
	DOWNTO: "downto", ELSE: "else", ELSIF: "elsif", END: "end", ENTITY: "entity",
	EXIT: "exit", FILE: "file", FOR: "for", FUNCTION: "function", GENERATE: "generate",
	GENERIC: "generic", GROUP: "group", GUARDED: "guarded", IF: "if", IMPURE: "impure",
	IN: "in", INERTIAL: "inertial", INOUT: "inout", IS: "is", LABEL: "label",
	LIBRARY: "library", LINKAGE: "linkage", LITERAL: "literal", LOOP: "loop",
	MAP: "map", MOD: "mod", NAND: "nand", NEW: "new", NEXT: "next", NOR: "nor",
	NOT: "not", NULL: "null", OF: "of", ON: "on", OPEN: "open", OR: "or",
	OTHERS: "others", OUT: "out", PACKAGE: "package", PORT: "port",
	POSTPONED: "postponed", PROCEDURE: "procedure", PROCESS: "process", PURE: "pure",
	RANGE: "range", RECORD: "record", REGISTER: "register", REJECT: "reject",
	REM: "rem", REPORT: "report", RETURN: "return", ROL: "rol", ROR: "ror",
	SELECT: "select", SEVERITY: "severity", SHARED: "shared", SIGNAL: "signal",
	SLA: "sla", SLL: "sll", SRA: "sra", SRL: "srl", SUBTYPE: "subtype", THEN: "then",
	TO: "to", TRANSPORT: "transport", TYPE: "type", UNAFFECTED: "unaffected",
	UNITS: "units", UNTIL: "until", USE: "use", VARIABLE: "variable", WAIT: "wait",
	WHEN: "when", WHILE: "while", WITH: "with", XNOR: "xnor", XOR: "xor",
}

// keywords maps lowercase reserved words to their Kind.
var keywords = func() map[string]Kind {
	m := make(map[string]Kind)
	for k := kwStart + 1; k < kwEnd; k++ {
		m[kindStr[k]] = k
	}
	return m
}()

// String returns the canonical text for a Kind.
func (k Kind) String() string {
	if s, ok := kindStr[k]; ok {
		return s
	}
	return fmt.Sprintf("Kind(%d)", int(k))
}

// LookupKeyword reports whether s (case-insensitively) is a reserved word.
func LookupKeyword(s string) (Kind, bool) {
	k, ok := keywords[lower(s)]
	return k, ok
}

func lower(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			b[i] = c + ('a' - 'A')
		}
	}
	return string(b)
}

// Pos is a source position.
type Pos struct {
	Line, Col, Offset int
}

// Token is a lexed token.
type Token struct {
	Kind Kind
	Lit  string
	Pos  Pos
}
