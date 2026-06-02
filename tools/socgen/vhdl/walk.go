package vhdl

import "fmt"

// A Visitor's Visit method is invoked for each node encountered by Walk. If the
// result visitor w is non-nil, Walk visits each of the children of node with w,
// then calls w.Visit(nil).
type Visitor interface {
	Visit(node Node) (w Visitor)
}

// Walk traverses an AST in depth-first order: it starts by calling
// v.Visit(node); node must not be nil. If the visitor w returned by
// v.Visit(node) is not nil, Walk is invoked recursively with visitor w for each
// of the non-nil children of node, followed by a call of w.Visit(nil).
func Walk(v Visitor, node Node) {
	if v = v.Visit(node); v == nil {
		return
	}
	switch n := node.(type) {
	// design file
	case *DesignFile:
		for _, c := range n.Context {
			Walk(v, c)
		}
		for _, u := range n.Units {
			Walk(v, u)
		}

	// context clauses (no child Nodes)
	case *LibraryClause:
	case *UseClause:

	// design units
	case *PackageDecl:
		for _, d := range n.Decls {
			Walk(v, d)
		}
	case *EntityDecl:
		for _, g := range n.Generics {
			Walk(v, g)
		}
		for _, p := range n.Ports {
			Walk(v, p)
		}

	// declarations
	case *ConstantDecl:
		if n.Constraint != nil {
			Walk(v, n.Constraint)
		}
		if n.Default != nil {
			Walk(v, n.Default)
		}
	case *SignalDecl:
		if n.Constraint != nil {
			Walk(v, n.Constraint)
		}
		if n.Default != nil {
			Walk(v, n.Default)
		}
	case *SubtypeDecl:
		if n.Constraint != nil {
			Walk(v, n.Constraint)
		}
	case *TypeDecl:
		if n.Def != nil {
			Walk(v, n.Def)
		}
	case *ComponentDecl:
		for _, g := range n.Generics {
			Walk(v, g)
		}
		for _, p := range n.Ports {
			Walk(v, p)
		}
	case *InterfaceDecl:
		if n.Constraint != nil {
			Walk(v, n.Constraint)
		}
		if n.Default != nil {
			Walk(v, n.Default)
		}

	// type definitions
	case *EnumDef:
	case *RecordDef:
		// RecordField is not a Node; descend into each field's constraint Expr.
		for _, f := range n.Fields {
			if f.Constraint != nil {
				Walk(v, f.Constraint)
			}
		}
	case *ArrayDef:

	// expressions
	case *BasicLit:
	case *Ident:
	case *Range:
		Walk(v, n.Left)
		Walk(v, n.Right)
	case *CallExpr:
		Walk(v, n.Fun)
		for _, a := range n.Args {
			Walk(v, a)
		}
	case *BinaryExpr:
		Walk(v, n.X)
		Walk(v, n.Y)
	case *UnaryExpr:
		Walk(v, n.X)
	case *ParenExpr:
		Walk(v, n.X)
	case *Aggregate:
		for _, e := range n.Elems {
			Walk(v, e)
		}
	case *ElementAssoc:
		for _, c := range n.Choices {
			Walk(v, c)
		}
		Walk(v, n.X)
	case *QualifiedExpr:
		Walk(v, n.Mark)
		Walk(v, n.X)

	default:
		// A node type with no case is a traversal gap. Panic loudly so it is
		// caught in tests rather than silently skipped.
		panic(fmt.Sprintf("vhdl.Walk: unexpected node type %T", node))
	}
	v.Visit(nil)
}

// inspector adapts a func(Node) bool to the Visitor interface.
type inspector func(Node) bool

func (f inspector) Visit(node Node) Visitor {
	if f(node) {
		return f
	}
	return nil
}

// Inspect traverses an AST in depth-first order: it calls f(node) for each
// node, recursing into children only while f returns true. It calls f(nil)
// after visiting a node's children (mirroring go/ast.Inspect).
func Inspect(node Node, f func(Node) bool) {
	Walk(inspector(f), node)
}
