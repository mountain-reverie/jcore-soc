package vhdl

import "fmt"

// ParseError is a single parse diagnostic at a source position.
type ParseError struct {
	Pos Position
	Msg string
}

func (e *ParseError) Error() string { return fmt.Sprintf("%s: %s", e.Pos, e.Msg) }

// CPPError is a C-preprocessor invocation failure.
type CPPError struct {
	Filename string
	CPP      string
	Err      error
}

func (e *CPPError) Error() string {
	return fmt.Sprintf("%s: cpp (%s) failed: %v", e.Filename, e.CPP, e.Err)
}
func (e *CPPError) Unwrap() error { return e.Err }
