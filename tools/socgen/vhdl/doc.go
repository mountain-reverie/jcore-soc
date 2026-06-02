// Package vhdl is a syntactic VHDL-93 frontend: lexer, parser, AST, and a
// canonical pretty-printer. It performs no name resolution or type checking;
// ambiguous prefix(args) is parsed structurally. Correctness is established by
// round-tripping the jcore corpus (parse -> print -> reparse -> equal AST).
package vhdl
