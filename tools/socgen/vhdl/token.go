package vhdl

import (
	"fmt"
	"sort"
)

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
	LBRACKET  // [
	RBRACKET  // ]
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

	LPAREN: "(", RPAREN: ")", LBRACKET: "[", RBRACKET: "]", COMMA: ",", SEMICOLON: ";", COLON: ":",
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
	// Fast path: VHDL source is overwhelmingly lowercase, so avoid allocating a
	// lowered copy unless an uppercase byte is actually present.
	if !hasUpper(s) {
		k, ok := keywords[s]
		return k, ok
	}
	k, ok := keywords[lower(s)]
	return k, ok
}

func hasUpper(s string) bool {
	for i := 0; i < len(s); i++ {
		if c := s[i]; c >= 'A' && c <= 'Z' {
			return true
		}
	}
	return false
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

// Pos is a compact encoding of a source position (a 1-based byte offset into a
// File within a FileSet). NoPos is the zero value.
type Pos int

const NoPos Pos = 0

// Position is a resolved, human-readable source position.
type Position struct {
	Filename string
	Offset   int // 0-based byte offset
	Line     int // 1-based
	Column   int // 1-based, in bytes
}

func (p Position) String() string {
	if p.Filename == "" {
		return fmt.Sprintf("%d:%d", p.Line, p.Column)
	}
	return fmt.Sprintf("%s:%d:%d", p.Filename, p.Line, p.Column)
}

// File tracks one source file's base offset and line starts.
type File struct {
	name  string
	base  int   // Pos of the first byte (1-based across the FileSet)
	size  int
	lines []int // byte offset of the start of each line (lines[0]==0)
}

func (f *File) AddLine(offset int) { f.lines = append(f.lines, offset) }
func (f *File) Pos(offset int) Pos { return Pos(f.base + offset) }
func (f *File) Position(p Pos) Position {
	off := int(p) - f.base
	// binary-search lines for the greatest start <= off
	i := sort.Search(len(f.lines), func(i int) bool { return f.lines[i] > off }) - 1
	if i < 0 {
		i = 0
	}
	return Position{Filename: f.name, Offset: off, Line: i + 1, Column: off - f.lines[i] + 1}
}

// FileSet maps Pos values back to Files.
type FileSet struct {
	base  int
	files []*File
}

func NewFileSet() *FileSet { return &FileSet{base: 1} }
func (s *FileSet) AddFile(name string, size int) *File {
	f := &File{name: name, base: s.base, size: size, lines: []int{0}}
	s.base += size + 1
	s.files = append(s.files, f)
	return f
}
func (s *FileSet) Position(p Pos) Position {
	for _, f := range s.files {
		if int(p) >= f.base && int(p) <= f.base+f.size {
			return f.Position(p)
		}
	}
	return Position{}
}

// Token is a lexed token.
type Token struct {
	Kind Kind
	Lit  string
	Pos  Pos
}
