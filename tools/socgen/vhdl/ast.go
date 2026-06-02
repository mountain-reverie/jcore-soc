package vhdl

type Node interface{ Pos() Pos }
type DesignUnit interface{ Node; isUnit() }
type Decl interface{ Node; isDecl() }
type Expr interface{ Node; isExpr() }
type TypeDef interface{ Node; isTypeDef() }

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

type LibraryClause struct{ P Pos; Names []string }
type UseClause     struct{ P Pos; Names []string }

func (n *LibraryClause) Pos() Pos { return n.P }
func (n *UseClause)     Pos() Pos { return n.P }

// design units
type PackageDecl struct{ P Pos; Name string; Decls []Decl }
type EntityDecl  struct{ P Pos; Name string; Generics []*InterfaceDecl; Ports []*InterfaceDecl }

func (n *PackageDecl) Pos() Pos { return n.P }
func (n *EntityDecl)  Pos() Pos { return n.P }
func (n *PackageDecl) isUnit()  {}
func (n *EntityDecl)  isUnit()  {}

// declarations
type ConstantDecl  struct{ P Pos; Names []string; SubtypeMark string; Constraint Expr; Default Expr }
type SignalDecl    struct{ P Pos; Names []string; SubtypeMark string; Constraint Expr; Default Expr }
type SubtypeDecl   struct{ P Pos; Name string; SubtypeMark string; Constraint Expr }
type TypeDecl      struct{ P Pos; Name string; Def TypeDef }
type ComponentDecl struct{ P Pos; Name string; Generics []*InterfaceDecl; Ports []*InterfaceDecl }
type InterfaceDecl struct{ P Pos; Names []string; Mode string; SubtypeMark string; Constraint Expr; Default Expr } // Mode: "" | "in" | "out" | "inout" | "buffer"

func (n *ConstantDecl)  Pos() Pos { return n.P }
func (n *SignalDecl)    Pos() Pos { return n.P }
func (n *SubtypeDecl)   Pos() Pos { return n.P }
func (n *TypeDecl)      Pos() Pos { return n.P }
func (n *ComponentDecl) Pos() Pos { return n.P }
func (n *InterfaceDecl) Pos() Pos { return n.P }

func (n *ConstantDecl)  isDecl() {}
func (n *SignalDecl)    isDecl() {}
func (n *SubtypeDecl)   isDecl() {}
func (n *TypeDecl)      isDecl() {}
func (n *ComponentDecl) isDecl() {}

// type definitions
type EnumDef   struct{ P Pos; Lits []string }
type RecordDef struct{ P Pos; Fields []RecordField }
type ArrayDef  struct{ P Pos; Text string }
type RecordField struct{ Names []string; SubtypeMark string; Constraint Expr } // not a Node

func (n *EnumDef)   Pos() Pos { return n.P }
func (n *RecordDef) Pos() Pos { return n.P }
func (n *ArrayDef)  Pos() Pos { return n.P }

func (n *EnumDef)   isTypeDef() {}
func (n *RecordDef) isTypeDef() {}
func (n *ArrayDef)  isTypeDef() {}

// expressions (minimal P1a set)
type Lit         struct{ P Pos; Text string }
type Name        struct{ P Pos; Text string }
type Range       struct{ P Pos; Left Expr; Dir string; Right Expr } // Dir: "to" | "downto"
type CallOrIndex struct{ P Pos; Prefix Expr; Args []Expr }
type BinaryExpr  struct{ P Pos; Op string; X, Y Expr }
type Paren       struct{ P Pos; X Expr }

func (n *Lit)         Pos() Pos { return n.P }
func (n *Name)        Pos() Pos { return n.P }
func (n *Range)       Pos() Pos { return n.P }
func (n *CallOrIndex) Pos() Pos { return n.P }
func (n *BinaryExpr)  Pos() Pos { return n.P }
func (n *Paren)       Pos() Pos { return n.P }

func (n *Lit)         isExpr() {}
func (n *Name)        isExpr() {}
func (n *Range)       isExpr() {}
func (n *CallOrIndex) isExpr() {}
func (n *BinaryExpr)  isExpr() {}
func (n *Paren)       isExpr() {}
