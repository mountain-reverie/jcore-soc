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
		for _, d := range n.Decls {
			Walk(v, d)
		}
	case *ArchitectureBody:
		for _, d := range n.Decls {
			Walk(v, d)
		}
		for _, s := range n.Stmts {
			Walk(v, s)
		}
	case *PackageBody:
		for _, d := range n.Decls {
			Walk(v, d)
		}
	case *ConfigurationDecl:
		for _, u := range n.Decls {
			Walk(v, u)
		}
		if n.Block != nil {
			Walk(v, n.Block)
		}
	case *BlockConfig:
		for _, u := range n.Uses {
			Walk(v, u)
		}
		for _, it := range n.Items {
			Walk(v, it)
		}
	case *ComponentConfig:
		if n.Binding != nil {
			Walk(v, n.Binding)
		}
		if n.Block != nil {
			Walk(v, n.Block)
		}
	case *BindingIndication:
		for _, e := range n.GenericMap {
			Walk(v, e)
		}
		for _, e := range n.PortMap {
			Walk(v, e)
		}

	// statements
	case *ConcurrentSignalAssign:
		Walk(v, n.Target)
		for _, el := range n.Waveform {
			if el.Value != nil {
				Walk(v, el.Value)
			}
			if el.After != nil {
				Walk(v, el.After)
			}
		}
		for _, c := range n.Conds {
			for _, el := range c.Waveform {
				if el.Value != nil {
					Walk(v, el.Value)
				}
				if el.After != nil {
					Walk(v, el.After)
				}
			}
			if c.Cond != nil {
				Walk(v, c.Cond)
			}
		}
	case *InstantiationStmt:
		for _, e := range n.GenericMap {
			Walk(v, e)
		}
		for _, e := range n.PortMap {
			Walk(v, e)
		}
	case *AssocElement:
		if n.Actual != nil {
			Walk(v, n.Actual)
		}
	case *GenerateStmt:
		if n.Range != nil {
			Walk(v, n.Range)
		}
		if n.Cond != nil {
			Walk(v, n.Cond)
		}
		for _, d := range n.Decls {
			Walk(v, d)
		}
		for _, s := range n.Stmts {
			Walk(v, s)
		}
	case *ProcessStmt:
		for _, s := range n.Sensitivity {
			Walk(v, s)
		}
		for _, d := range n.Decls {
			Walk(v, d)
		}
		for _, s := range n.Stmts {
			Walk(v, s)
		}
	case *SignalAssignStmt:
		Walk(v, n.Target)
		for _, el := range n.Waveform {
			if el.Value != nil {
				Walk(v, el.Value)
			}
			if el.After != nil {
				Walk(v, el.After)
			}
		}
	case *VariableAssignStmt:
		Walk(v, n.Target)
		if n.Value != nil {
			Walk(v, n.Value)
		}
	case *NullStmt:
		// no child nodes
	case *ReturnStmt:
		if n.Value != nil {
			Walk(v, n.Value)
		}
	case *CaseStmt:
		if n.Expr != nil {
			Walk(v, n.Expr)
		}
		for _, alt := range n.Alts {
			for _, c := range alt.Choices {
				Walk(v, c)
			}
			for _, s := range alt.Stmts {
				Walk(v, s)
			}
		}
	case *IfStmt:
		if n.Cond != nil {
			Walk(v, n.Cond)
		}
		for _, s := range n.Then {
			Walk(v, s)
		}
		for _, ei := range n.Elsifs {
			if ei.Cond != nil {
				Walk(v, ei.Cond)
			}
			for _, s := range ei.Stmts {
				Walk(v, s)
			}
		}
		for _, s := range n.Else {
			Walk(v, s)
		}
	case *LoopStmt:
		if n.Range != nil {
			Walk(v, n.Range)
		}
		if n.Cond != nil {
			Walk(v, n.Cond)
		}
		for _, s := range n.Stmts {
			Walk(v, s)
		}
	case *NextStmt:
		if n.When != nil {
			Walk(v, n.When)
		}
	case *ExitStmt:
		if n.When != nil {
			Walk(v, n.When)
		}
	case *AssertStmt:
		if n.Cond != nil {
			Walk(v, n.Cond)
		}
		if n.Report != nil {
			Walk(v, n.Report)
		}
		if n.Severity != nil {
			Walk(v, n.Severity)
		}
	case *ReportStmt:
		if n.Report != nil {
			Walk(v, n.Report)
		}
		if n.Severity != nil {
			Walk(v, n.Severity)
		}
	case *WaitStmt:
		for _, s := range n.On {
			Walk(v, s)
		}
		if n.Until != nil {
			Walk(v, n.Until)
		}
		if n.For != nil {
			Walk(v, n.For)
		}

	case *ProcedureCallStmt:
		for _, a := range n.Args {
			Walk(v, a)
		}
	case *SelectedSignalAssign:
		if n.Expr != nil {
			Walk(v, n.Expr)
		}
		if n.Target != nil {
			Walk(v, n.Target)
		}
		for _, alt := range n.Alts {
			for _, el := range alt.Waveform {
				if el.Value != nil {
					Walk(v, el.Value)
				}
				if el.After != nil {
					Walk(v, el.After)
				}
			}
			for _, c := range alt.Choices {
				Walk(v, c)
			}
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
	case *VariableDecl:
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
	case *SubprogramDecl:
		for _, prm := range n.Params {
			Walk(v, prm)
		}
	case *SubprogramBody:
		for _, prm := range n.Params {
			Walk(v, prm)
		}
		for _, d := range n.Decls {
			Walk(v, d)
		}
		for _, s := range n.Stmts {
			Walk(v, s)
		}
	case *AliasDecl:
		if n.Constraint != nil {
			Walk(v, n.Constraint)
		}
		if n.Target != nil {
			Walk(v, n.Target)
		}
	case *ConfigSpec:
		if n.Binding != nil {
			Walk(v, n.Binding)
		}
	case *AttributeDecl:
		// no child nodes
	case *GroupTemplateDecl:
		// no child nodes
	case *GroupDecl:
		// no child nodes
	case *AttributeSpec:
		if n.Value != nil {
			Walk(v, n.Value)
		}
	case *FileDecl:
		if n.OpenMode != nil {
			Walk(v, n.OpenMode)
		}
		if n.LogicalName != nil {
			Walk(v, n.LogicalName)
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
	case *FileTypeDef:
		// no child nodes
	case *AccessDef:
		// no child nodes

	// expressions
	case *PhysicalLit:
		// no child nodes
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
	case *SelectorExpr:
		Walk(v, n.X)
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
	case *RangeConstraint:
		Walk(v, n.Mark)
		Walk(v, n.Range)

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
