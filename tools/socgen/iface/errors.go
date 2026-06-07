package iface

import (
	"errors"
	"fmt"
)

var (
	ErrDuplicateSymbol = errors.New("duplicate symbol")
	ErrDuplicateDecl   = errors.New("duplicate declaration")
)

// DuplicateError reports a duplicated declaration or symbol found during extraction.
type DuplicateError struct {
	Kind   error  // ErrDuplicateSymbol | ErrDuplicateDecl (the category sentinel)
	Symbol string
	Decl   string // declaration kind (entity/package/...) — for ErrDuplicateDecl
	Pkg    string // for ErrDuplicateSymbol
	AlsoIn string // for ErrDuplicateSymbol
}

func (e *DuplicateError) Error() string {
	if errors.Is(e.Kind, ErrDuplicateDecl) {
		return fmt.Sprintf("duplicate %s declaration: %s", e.Decl, e.Symbol)
	}
	return fmt.Sprintf("duplicate symbol %q in package %s (also in %s)", e.Symbol, e.Pkg, e.AlsoIn)
}

func (e *DuplicateError) Unwrap() error { return e.Kind }
