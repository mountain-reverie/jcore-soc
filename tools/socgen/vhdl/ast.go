package vhdl

type Node interface {
	Pos() Pos // position of the first character of the node
	End() Pos // position immediately after the last character of the node
}
type DesignUnit interface{ Node; unitNode() }
type Decl interface{ Node; declNode() }
type Expr interface{ Node; exprNode() }
type TypeDef interface{ Node; typeDefNode() }
type Stmt interface{ Node; stmtNode() }

type DesignFile struct {
	Context []Node // *LibraryClause / *UseClause
	Units   []DesignUnit
}

func (n *DesignFile) Pos() Pos {
	if len(n.Units) > 0 {
		return n.Units[0].Pos()
	}
	return NoPos
}

// End positions for declarations and clauses are best-effort in P1b: where a
// node tracks no closing token, End falls back to the end of its last child or
// to its start. Positions are not consulted by the round-trip oracle (equalAST
// ignores them); precise declaration End tracking is a later refinement.
func (n *DesignFile) End() Pos {
	if k := len(n.Units); k > 0 { return n.Units[k-1].End() }
	if k := len(n.Context); k > 0 { return n.Context[k-1].End() }
	return NoPos
}

type LibraryClause struct{ P Pos; Names []string }
type UseClause     struct{ P Pos; Names []string }

func (n *LibraryClause) Pos() Pos { return n.P }
func (n *UseClause)     Pos() Pos { return n.P }

func (n *LibraryClause) End() Pos { return n.P }
func (n *UseClause)     End() Pos { return n.P }

// design units
type PackageDecl struct{ P Pos; Name string; Decls []Decl }
type EntityDecl  struct{ P Pos; Name string; Generics []*InterfaceDecl; Ports []*InterfaceDecl; Decls []Decl }

func (n *PackageDecl) Pos() Pos { return n.P }
func (n *EntityDecl)  Pos() Pos { return n.P }
func (n *PackageDecl) unitNode()  {}
func (n *EntityDecl)  unitNode()  {}

func (n *PackageDecl) End() Pos { if k := len(n.Decls); k > 0 { return n.Decls[k-1].End() }; return n.P }
func (n *EntityDecl)  End() Pos {
	if k := len(n.Decls); k > 0 { return n.Decls[k-1].End() }
	if k := len(n.Ports); k > 0 { return n.Ports[k-1].End() }
	if k := len(n.Generics); k > 0 { return n.Generics[k-1].End() }
	return n.P
}

// ArchitectureBody is `architecture name of entity is <decls> begin <stmts> end;`.
type ArchitectureBody struct {
	P      Pos
	Name   string
	Entity string
	Decls  []Decl
	Stmts  []Stmt
}

func (n *ArchitectureBody) Pos() Pos { return n.P }
func (n *ArchitectureBody) End() Pos {
	if k := len(n.Stmts); k > 0 { return n.Stmts[k-1].End() }
	if k := len(n.Decls); k > 0 { return n.Decls[k-1].End() }
	return n.P
}
func (n *ArchitectureBody) unitNode() {}

// CondWaveform is one `value when cond` arm of a conditional signal assignment.
// The final `else value` arm has Cond == nil. (Populated in a later task.)
type CondWaveform struct {
	Value Expr
	Cond  Expr
}

// ConcurrentSignalAssign is `[label:] target <= ... ;`. A simple assignment sets
// Waveform (Conds nil); a conditional assignment sets Conds (Waveform nil).
type ConcurrentSignalAssign struct {
	P        Pos
	Label    string
	Target   Expr
	Waveform Expr
	Conds    []*CondWaveform
}

func (n *ConcurrentSignalAssign) Pos() Pos { return n.P }
func (n *ConcurrentSignalAssign) End() Pos {
	if n.Waveform != nil { return n.Waveform.End() }
	if k := len(n.Conds); k > 0 && n.Conds[k-1].Value != nil { return n.Conds[k-1].Value.End() }
	return n.Target.End()
}
func (n *ConcurrentSignalAssign) stmtNode() {}

// declarations
type ConstantDecl  struct{ P Pos; Names []string; SubtypeMark string; Constraint Expr; Default Expr }
type SignalDecl    struct{ P Pos; Names []string; SubtypeMark string; Constraint Expr; Default Expr }
type SubtypeDecl   struct{ P Pos; Name string; SubtypeMark string; Constraint Expr }
type TypeDecl      struct{ P Pos; Name string; Def TypeDef }
type ComponentDecl struct{ P Pos; Name string; Generics []*InterfaceDecl; Ports []*InterfaceDecl }
type InterfaceDecl struct{ P Pos; Names []string; Mode string; SubtypeMark string; Constraint Expr; Default Expr } // Mode: "" | "in" | "out" | "inout" | "buffer"

// SubprogramDecl is a function/procedure specification (declaration). Bodies are
// deferred. Designator is an identifier or a string-literal operator symbol.
type SubprogramDecl struct {
	P           Pos
	IsProcedure bool
	Pure        bool
	Impure      bool
	Designator  string
	Params      []*InterfaceDecl
	ReturnMark  string // function return type mark; "" for procedures
}

func (n *ConstantDecl)   Pos() Pos { return n.P }
func (n *SignalDecl)     Pos() Pos { return n.P }
func (n *SubtypeDecl)    Pos() Pos { return n.P }
func (n *TypeDecl)       Pos() Pos { return n.P }
func (n *ComponentDecl)  Pos() Pos { return n.P }
func (n *InterfaceDecl)  Pos() Pos { return n.P }
func (n *SubprogramDecl) Pos() Pos { return n.P }

func (n *ConstantDecl)   declNode() {}
func (n *SignalDecl)     declNode() {}
func (n *SubtypeDecl)    declNode() {}
func (n *TypeDecl)       declNode() {}
func (n *ComponentDecl)  declNode() {}
func (n *SubprogramDecl) declNode() {}

func (n *ConstantDecl) End() Pos {
	if n.Default != nil { return n.Default.End() }
	if n.Constraint != nil { return n.Constraint.End() }
	return n.P
}
func (n *SignalDecl) End() Pos {
	if n.Default != nil { return n.Default.End() }
	if n.Constraint != nil { return n.Constraint.End() }
	return n.P
}
func (n *SubtypeDecl) End() Pos { if n.Constraint != nil { return n.Constraint.End() }; return n.P }
func (n *TypeDecl)    End() Pos { if n.Def != nil { return n.Def.End() }; return n.P }
func (n *ComponentDecl) End() Pos {
	if k := len(n.Ports); k > 0 { return n.Ports[k-1].End() }
	if k := len(n.Generics); k > 0 { return n.Generics[k-1].End() }
	return n.P
}
func (n *SubprogramDecl) End() Pos {
	if k := len(n.Params); k > 0 {
		return n.Params[k-1].End()
	}
	return n.P
}

// AliasDecl is `alias name [: subtype_indication] is target ;`. SubtypeMark is ""
// when no subtype indication is present. Target is the aliased name expression.
type AliasDecl struct {
	P           Pos
	Name        string
	SubtypeMark string
	Constraint  Expr
	Target      Expr
}

func (n *AliasDecl) Pos() Pos { return n.P }
func (n *AliasDecl) End() Pos {
	if n.Target != nil {
		return n.Target.End()
	}
	return n.P
}
func (n *AliasDecl) declNode() {}

// AttributeDecl is `attribute name : type_mark ;`.
type AttributeDecl struct{ P Pos; Name, TypeMark string }

// AttributeSpec is `attribute name of <entities> : <class> is value ;`.
// EntityClass is the entity-class keyword Kind (SIGNAL, SUBTYPE, VARIABLE, ...).
type AttributeSpec struct {
	P           Pos
	Name        string
	Entities    []string
	EntityClass Kind
	Value       Expr
}

func (n *AttributeDecl) Pos() Pos { return n.P }
func (n *AttributeDecl) End() Pos { return n.P }
func (n *AttributeDecl) declNode() {}

// GroupTemplateDecl is `group name is (class [<>] {, class [<>]}) ;`.
// Each Classes entry is the class keyword, with " <>" appended when boxed.
type GroupTemplateDecl struct{ P Pos; Name string; Classes []string }

// GroupDecl is `group name : template_name (constituent {, constituent}) ;`.
type GroupDecl struct{ P Pos; Name string; TemplateMark string; Constituents []string }

func (n *GroupTemplateDecl) Pos() Pos { return n.P }
func (n *GroupTemplateDecl) End() Pos { return n.P }
func (n *GroupTemplateDecl) declNode() {}

func (n *GroupDecl) Pos() Pos { return n.P }
func (n *GroupDecl) End() Pos { return n.P }
func (n *GroupDecl) declNode() {}

func (n *AttributeSpec) Pos() Pos { return n.P }
func (n *AttributeSpec) End() Pos {
	if n.Value != nil {
		return n.Value.End()
	}
	return n.P
}
func (n *AttributeSpec) declNode() {}

func (n *InterfaceDecl) End() Pos {
	if n.Default != nil { return n.Default.End() }
	if n.Constraint != nil { return n.Constraint.End() }
	return n.P
}

// type definitions
type EnumDef   struct{ P Pos; Lits []string }
type RecordDef struct{ P Pos; Fields []RecordField }
type ArrayDef  struct{ P Pos; Text string }
type RecordField struct{ Names []string; SubtypeMark string; Constraint Expr } // not a Node

func (n *EnumDef)   Pos() Pos { return n.P }
func (n *RecordDef) Pos() Pos { return n.P }
func (n *ArrayDef)  Pos() Pos { return n.P }

func (n *EnumDef)   typeDefNode() {}
func (n *RecordDef) typeDefNode() {}
func (n *ArrayDef)  typeDefNode() {}

func (n *EnumDef)   End() Pos { return n.P }
func (n *RecordDef) End() Pos { return n.P }
func (n *ArrayDef)  End() Pos { return n.P }

// expressions
type BasicLit    struct{ ValuePos Pos; Kind Kind; Value string } // INT/REAL/BASEDLIT/CHARLIT/STRINGLIT/BITSTRINGLIT
type Ident       struct{ NamePos Pos; Name string }              // a (possibly compound/attributed) name; full decomposition into SelectorExpr is deferred
type Range       struct{ Left Expr; DirPos Pos; Dir Kind; Right Expr } // Dir is TO or DOWNTO
type CallExpr     struct{ Fun Expr; Lparen Pos; Args []Expr; Rparen Pos } // also indexed-name / slice / type-conversion — VHDL can't disambiguate syntactically
type BinaryExpr  struct{ X Expr; OpPos Pos; Op Kind; Y Expr }
type UnaryExpr   struct{ OpPos Pos; Op Kind; X Expr } // abs / not / unary + / unary -
type ParenExpr   struct{ Lparen Pos; X Expr; Rparen Pos }

// Aggregate is a parenthesized element-association list: (e1, choice => e2, ...).
type Aggregate struct{ Lparen Pos; Elems []*ElementAssoc; Rparen Pos }

// ElementAssoc is one element of an Aggregate. Choices == nil means a positional
// element; otherwise it is `choice {| choice} => X`. (`others` appears as an
// *Ident in Choices.) ElementAssoc is a Node but not an Expr — it appears only
// inside Aggregate.Elems.
type ElementAssoc struct{ Choices []Expr; ArrowPos Pos; X Expr }

func (n *Aggregate)    Pos() Pos { return n.Lparen }
func (n *Aggregate)    End() Pos { return n.Rparen + 1 }
func (n *Aggregate)    exprNode() {}

func (n *ElementAssoc) Pos() Pos {
	if len(n.Choices) > 0 {
		return n.Choices[0].Pos()
	}
	return n.X.Pos()
}
func (n *ElementAssoc) End() Pos { return n.X.End() }

func (n *BasicLit)   Pos() Pos { return n.ValuePos }
func (n *Ident)      Pos() Pos { return n.NamePos }
func (n *Range)      Pos() Pos { return n.Left.Pos() }
func (n *CallExpr)   Pos() Pos { return n.Fun.Pos() }
func (n *BinaryExpr) Pos() Pos { return n.X.Pos() }
func (n *UnaryExpr)  Pos() Pos { return n.OpPos }
func (n *ParenExpr)  Pos() Pos { return n.Lparen }

func (n *BasicLit)   End() Pos { return n.ValuePos + Pos(len(n.Value)) }
func (n *Ident)      End() Pos { return n.NamePos + Pos(len(n.Name)) }
func (n *Range)      End() Pos { return n.Right.End() }
func (n *CallExpr)   End() Pos { return n.Rparen + 1 }
func (n *BinaryExpr) End() Pos { return n.Y.End() }
func (n *UnaryExpr)  End() Pos { return n.X.End() }
func (n *ParenExpr)  End() Pos { return n.Rparen + 1 }

// QualifiedExpr is a qualified expression: Mark'(X), where X is a *ParenExpr or
// *Aggregate. (The tick distinguishes it from an attribute name and a char literal.)
// Tick is the apostrophe's position, retained for diagnostics.
type QualifiedExpr struct{ Mark Expr; Tick Pos; X Expr }

func (n *QualifiedExpr) Pos() Pos { return n.Mark.Pos() }
func (n *QualifiedExpr) End() Pos { return n.X.End() }
func (n *QualifiedExpr) exprNode() {}

func (n *BasicLit)   exprNode() {}
func (n *Ident)      exprNode() {}
func (n *Range)      exprNode() {}
func (n *CallExpr)   exprNode() {}
func (n *BinaryExpr) exprNode() {}
func (n *UnaryExpr)  exprNode() {}
func (n *ParenExpr)  exprNode() {}
