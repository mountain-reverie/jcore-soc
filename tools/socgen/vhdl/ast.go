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

func (n *UseClause) declNode() {}

// design units
type PackageDecl struct{ P Pos; Name string; Decls []Decl }
type EntityDecl  struct{ P Pos; Name string; Generics []*InterfaceDecl; Ports []*InterfaceDecl; Decls []Decl; Stmts []Stmt }

func (n *PackageDecl) Pos() Pos { return n.P }
func (n *EntityDecl)  Pos() Pos { return n.P }
func (n *PackageDecl) unitNode()  {}
func (n *EntityDecl)  unitNode()  {}

func (n *PackageDecl) End() Pos { if k := len(n.Decls); k > 0 { return n.Decls[k-1].End() }; return n.P }
func (n *EntityDecl)  End() Pos {
	if k := len(n.Stmts); k > 0 { return n.Stmts[k-1].End() }
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

// PackageBody is `package body name is <declarative_items> end package body ;`.
type PackageBody struct{ P Pos; Name string; Decls []Decl }

func (n *PackageBody) Pos() Pos { return n.P }
func (n *PackageBody) End() Pos {
	if k := len(n.Decls); k > 0 { return n.Decls[k-1].End() }
	return n.P
}
func (n *PackageBody) unitNode() {}

// ConfigurationDecl is `configuration name of entity is <decls> <block> end ;`.
type ConfigurationDecl struct {
	P      Pos
	Name   string
	Entity string
	Decls  []*UseClause // configuration declarative part (use clauses)
	Block  *BlockConfig // the top block configuration
}

func (n *ConfigurationDecl) Pos() Pos { return n.P }
func (n *ConfigurationDecl) End() Pos {
	if n.Block != nil { return n.Block.End() }
	if k := len(n.Decls); k > 0 { return n.Decls[k-1].End() }
	return n.P
}
func (n *ConfigurationDecl) unitNode() {}

// BlockConfig is `for spec <uses> <items> end for ;`. Items are *BlockConfig
// (nested block) or *ComponentConfig (added in Task 2).
type BlockConfig struct {
	P     Pos
	Spec  string // architecture name (top) or block/generate label (nested)
	Uses  []*UseClause
	Items []Node
}

func (n *BlockConfig) Pos() Pos { return n.P }
func (n *BlockConfig) End() Pos {
	if k := len(n.Items); k > 0 { return n.Items[k-1].End() }
	return n.P
}

// ComponentConfig is `for inst_list : comp [binding;] [block_config] end for ;`.
type ComponentConfig struct {
	P       Pos
	Insts   []string // instantiation labels, or "all" / "others"
	Comp    string
	Binding *BindingIndication
	Block   *BlockConfig
}

func (n *ComponentConfig) Pos() Pos { return n.P }
func (n *ComponentConfig) End() Pos {
	if n.Block != nil { return n.Block.End() }
	if n.Binding != nil { return n.Binding.End() }
	return n.P
}

// ConfigSpec is a configuration specification (a declarative item):
// `for inst_list : comp binding_indication ;`.
type ConfigSpec struct {
	P       Pos
	Insts   []string // instantiation labels, or "all" / "others"
	Comp    string
	Binding *BindingIndication
}

func (n *ConfigSpec) Pos() Pos { return n.P }
func (n *ConfigSpec) End() Pos {
	if n.Binding != nil { return n.Binding.End() }
	return n.P
}
func (n *ConfigSpec) declNode() {}

// BindingIndication is `use (entity name[(arch)] | configuration name | open)
// [generic map (...)] [port map (...)]`.
type BindingIndication struct {
	P          Pos
	UnitKind   Kind // ENTITY | CONFIGURATION | OPEN
	Unit       string
	Arch       string
	GenericMap []*AssocElement
	PortMap    []*AssocElement
}

func (n *BindingIndication) Pos() Pos { return n.P }
func (n *BindingIndication) End() Pos {
	if k := len(n.PortMap); k > 0 { return n.PortMap[k-1].End() }
	if k := len(n.GenericMap); k > 0 { return n.GenericMap[k-1].End() }
	return n.P
}

// CondWaveform is one `waveform when cond` arm of a conditional signal
// assignment. The final bare `else waveform` arm has Cond == nil.
type CondWaveform struct {
	Waveform []*WaveformElem
	Cond     Expr
}

// WaveformElem is one element of a signal-assignment waveform: `value [after time]`.
type WaveformElem struct {
	Value Expr
	After Expr // nil if no `after` clause
}

// SelectedWaveform is one `waveform when choices` alternative of a selected
// signal assignment.
type SelectedWaveform struct {
	Waveform []*WaveformElem
	Choices  []Expr // an `others` choice is an *Ident
}

// DelayMechanism is a signal-assignment delay: `transport`, `inertial`, or
// `reject <time> inertial`. Plain struct (not a Node); walked via its assign node.
type DelayMechanism struct {
	Transport bool // true => transport; false => inertial
	Reject    Expr // reject time (nil unless `reject <expr> inertial`)
}

// SelectedSignalAssign is `with expr select target <= { waveform when choices , } ;`.
type SelectedSignalAssign struct {
	P      Pos
	Label  string
	Expr   Expr
	Target Expr
	Delay  *DelayMechanism
	Alts   []*SelectedWaveform
}

func (n *SelectedSignalAssign) Pos() Pos { return n.P }
func (n *SelectedSignalAssign) End() Pos {
	if k := len(n.Alts); k > 0 {
		if m := len(n.Alts[k-1].Choices); m > 0 {
			return n.Alts[k-1].Choices[m-1].End()
		}
	}
	if n.Target != nil {
		return n.Target.End()
	}
	return n.P
}
func (n *SelectedSignalAssign) stmtNode() {}

// ConcurrentSignalAssign is `[label:] target <= ... ;`. A simple assignment sets
// Waveform (Conds nil); a conditional assignment sets Conds (Waveform nil).
type ConcurrentSignalAssign struct {
	P        Pos
	Label    string
	Target   Expr
	Delay    *DelayMechanism
	Waveform []*WaveformElem
	Conds    []*CondWaveform
}

func (n *ConcurrentSignalAssign) Pos() Pos { return n.P }
func (n *ConcurrentSignalAssign) End() Pos {
	if k := len(n.Waveform); k > 0 {
		last := n.Waveform[k-1]
		if last.After != nil { return last.After.End() }
		if last.Value != nil { return last.Value.End() }
	}
	if k := len(n.Conds); k > 0 {
		if w := n.Conds[k-1].Waveform; len(w) > 0 {
			last := w[len(w)-1]
			if last.After != nil { return last.After.End() }
			if last.Value != nil { return last.Value.End() }
		}
	}
	return n.Target.End()
}
func (n *ConcurrentSignalAssign) stmtNode() {}

// AssocElement is one association `[formal =>] actual` in a generic/port map.
// Formal == "" means a positional association. Actual may be Ident{"open"}.
type AssocElement struct {
	P      Pos
	Formal string
	Actual Expr
}

func (n *AssocElement) Pos() Pos { return n.P }
func (n *AssocElement) End() Pos {
	if n.Actual != nil {
		return n.Actual.End()
	}
	return n.P
}

// InstantiationStmt is a component/entity/configuration instantiation:
//
//	label : [entity|component|configuration] unit [(arch)] [generic map(...)] [port map(...)] ;
//
// UnitKind is ENTITY/COMPONENT/CONFIGURATION, or 0 for a bare component instance.
type InstantiationStmt struct {
	P          Pos
	Label      string
	UnitKind   Kind
	Unit       string
	Arch       string // entity architecture spec: entity work.x(arch)
	GenericMap []*AssocElement
	PortMap    []*AssocElement
}

func (n *InstantiationStmt) Pos() Pos { return n.P }
func (n *InstantiationStmt) End() Pos {
	if k := len(n.PortMap); k > 0 {
		return n.PortMap[k-1].End()
	}
	if k := len(n.GenericMap); k > 0 {
		return n.GenericMap[k-1].End()
	}
	return n.P
}
func (n *InstantiationStmt) stmtNode() {}

// ProcedureCallStmt is `[label:] name [(actual {, actual})] ;` (sequential or concurrent).
type ProcedureCallStmt struct {
	P     Pos
	Label string
	Name  string
	Args  []*AssocElement
}

func (n *ProcedureCallStmt) Pos() Pos { return n.P }
func (n *ProcedureCallStmt) End() Pos {
	if k := len(n.Args); k > 0 {
		return n.Args[k-1].End()
	}
	return n.P
}
func (n *ProcedureCallStmt) stmtNode() {}

// GenerateStmt is `label : (for id in range | if cond) generate [decls begin] stmts end generate ;`.
type GenerateStmt struct {
	P     Pos
	Label string
	Kind  Kind   // FOR or IF
	Param string // for-generate loop parameter ("" for if-generate)
	Range Expr   // for-generate discrete range
	Cond  Expr   // if-generate condition
	Decls []Decl
	Stmts []Stmt
}

func (n *GenerateStmt) Pos() Pos { return n.P }
func (n *GenerateStmt) End() Pos {
	if k := len(n.Stmts); k > 0 {
		return n.Stmts[k-1].End()
	}
	if k := len(n.Decls); k > 0 {
		return n.Decls[k-1].End()
	}
	return n.P
}
func (n *GenerateStmt) stmtNode() {}

// VariableDecl is `[shared] variable names : subtype [:= default] ;` (process/subprogram local,
// or shared variable declared in a protected type or package body).
type VariableDecl struct{ P Pos; Shared bool; Names []string; SubtypeMark string; Constraint Expr; Default Expr }

func (n *VariableDecl) Pos() Pos { return n.P }
func (n *VariableDecl) End() Pos {
	if n.Default != nil { return n.Default.End() }
	if n.Constraint != nil { return n.Constraint.End() }
	return n.P
}
func (n *VariableDecl) declNode() {}

// ProcessStmt is `[postponed] process [(sensitivity)] [is] <decls> begin <stmts> end process ;`.
type ProcessStmt struct {
	P           Pos
	Label       string
	Postponed   bool
	Sensitivity []Expr
	Decls       []Decl
	Stmts       []Stmt
}

func (n *ProcessStmt) Pos() Pos { return n.P }
func (n *ProcessStmt) End() Pos {
	if k := len(n.Stmts); k > 0 { return n.Stmts[k-1].End() }
	if k := len(n.Decls); k > 0 { return n.Decls[k-1].End() }
	return n.P
}
func (n *ProcessStmt) stmtNode() {}

// SignalAssignStmt is a sequential `[label:] target <= waveform ;`.
type SignalAssignStmt struct {
	P        Pos
	Label    string
	Target   Expr
	Delay    *DelayMechanism
	Waveform []*WaveformElem
}

func (n *SignalAssignStmt) Pos() Pos { return n.P }
func (n *SignalAssignStmt) End() Pos {
	if k := len(n.Waveform); k > 0 {
		last := n.Waveform[k-1]
		if last.After != nil { return last.After.End() }
		if last.Value != nil { return last.Value.End() }
	}
	return n.Target.End()
}
func (n *SignalAssignStmt) stmtNode() {}

// VariableAssignStmt is `[label:] target := value ;`.
type VariableAssignStmt struct{ P Pos; Label string; Target Expr; Value Expr }

func (n *VariableAssignStmt) Pos() Pos { return n.P }
func (n *VariableAssignStmt) End() Pos { if n.Value != nil { return n.Value.End() }; return n.Target.End() }
func (n *VariableAssignStmt) stmtNode() {}

// ElsifClause is one `elsif cond then <stmts>` arm of an IfStmt.
type ElsifClause struct {
	Cond  Expr
	Stmts []Stmt
}

// IfStmt is `[label:] if cond then <then> {elsif cond then <stmts>} [else <else>] end if ;`.
type IfStmt struct {
	P      Pos
	Label  string
	Cond   Expr
	Then   []Stmt
	Elsifs []*ElsifClause
	Else   []Stmt
}

func (n *IfStmt) Pos() Pos { return n.P }
func (n *IfStmt) End() Pos {
	if k := len(n.Else); k > 0 { return n.Else[k-1].End() }
	if k := len(n.Elsifs); k > 0 {
		if m := len(n.Elsifs[k-1].Stmts); m > 0 { return n.Elsifs[k-1].Stmts[m-1].End() }
	}
	if k := len(n.Then); k > 0 { return n.Then[k-1].End() }
	return n.P
}
func (n *IfStmt) stmtNode() {}

// CaseAlt is one `when choices => <stmts>` alternative of a CaseStmt.
type CaseAlt struct {
	Choices []Expr // an `others` choice is an *Ident{Name:"others"}
	Stmts   []Stmt
}

// CaseStmt is `[label:] case expr is { when choices => <stmts> } end case ;`.
type CaseStmt struct {
	P     Pos
	Label string
	Expr  Expr
	Alts  []*CaseAlt
}

func (n *CaseStmt) Pos() Pos { return n.P }
func (n *CaseStmt) End() Pos {
	if k := len(n.Alts); k > 0 {
		if m := len(n.Alts[k-1].Stmts); m > 0 { return n.Alts[k-1].Stmts[m-1].End() }
	}
	return n.P
}
func (n *CaseStmt) stmtNode() {}

// NullStmt is `[label:] null ;`.
type NullStmt struct{ P Pos; Label string }

func (n *NullStmt) Pos() Pos { return n.P }
func (n *NullStmt) End() Pos { return n.P }
func (n *NullStmt) stmtNode() {}

// ReturnStmt is `[label:] return [expression] ;`. Value is nil for a bare return.
type ReturnStmt struct{ P Pos; Label string; Value Expr }

func (n *ReturnStmt) Pos() Pos { return n.P }
func (n *ReturnStmt) End() Pos { if n.Value != nil { return n.Value.End() }; return n.P }
func (n *ReturnStmt) stmtNode() {}

// LoopStmt is a loop statement. Scheme is FOR (Param+Range set), WHILE (Cond
// set), or 0 for a bare loop.
type LoopStmt struct {
	P      Pos
	Label  string
	Scheme Kind // FOR, WHILE, or 0 (zero value) = bare loop (no scheme)
	Param  string
	Range  Expr
	Cond   Expr // while-loop condition
	Stmts  []Stmt
}

func (n *LoopStmt) Pos() Pos { return n.P }
func (n *LoopStmt) End() Pos {
	if k := len(n.Stmts); k > 0 { return n.Stmts[k-1].End() }
	return n.P
}
func (n *LoopStmt) stmtNode() {}

// NextStmt is `[label:] next [loop_label] [when cond] ;`.
type NextStmt struct{ P Pos; Label string; LoopLabel string; When Expr }

func (n *NextStmt) Pos() Pos { return n.P }
func (n *NextStmt) End() Pos { if n.When != nil { return n.When.End() }; return n.P }
func (n *NextStmt) stmtNode() {}

// ExitStmt is `[label:] exit [loop_label] [when cond] ;`.
type ExitStmt struct{ P Pos; Label string; LoopLabel string; When Expr }

func (n *ExitStmt) Pos() Pos { return n.P }
func (n *ExitStmt) End() Pos { if n.When != nil { return n.When.End() }; return n.P }
func (n *ExitStmt) stmtNode() {}

// AssertStmt is `[label:] assert cond [report expr] [severity expr] ;` (sequential or concurrent).
type AssertStmt struct {
	P        Pos
	Label    string
	Cond     Expr
	Report   Expr
	Severity Expr
}

func (n *AssertStmt) Pos() Pos { return n.P }
func (n *AssertStmt) End() Pos {
	if n.Severity != nil { return n.Severity.End() }
	if n.Report != nil { return n.Report.End() }
	if n.Cond != nil { return n.Cond.End() }
	return n.P
}
func (n *AssertStmt) stmtNode() {}

// ReportStmt is `[label:] report expr [severity expr] ;` (sequential).
type ReportStmt struct {
	P        Pos
	Label    string
	Report   Expr
	Severity Expr
}

func (n *ReportStmt) Pos() Pos { return n.P }
func (n *ReportStmt) End() Pos {
	if n.Severity != nil { return n.Severity.End() }
	if n.Report != nil { return n.Report.End() }
	return n.P
}
func (n *ReportStmt) stmtNode() {}

// WaitStmt is `[label:] wait [on signals] [until cond] [for time] ;`.
type WaitStmt struct {
	P     Pos
	Label string
	On    []Expr // sensitivity clause signal names
	Until Expr   // condition clause
	For   Expr   // timeout clause
}

func (n *WaitStmt) Pos() Pos { return n.P }
func (n *WaitStmt) End() Pos {
	if n.For != nil { return n.For.End() }
	if n.Until != nil { return n.Until.End() }
	if k := len(n.On); k > 0 { return n.On[k-1].End() }
	return n.P
}
func (n *WaitStmt) stmtNode() {}

// declarations
type ConstantDecl  struct{ P Pos; Names []string; SubtypeMark string; Constraint Expr; Default Expr }
type SignalDecl    struct{ P Pos; Names []string; SubtypeMark string; Constraint Expr; Default Expr }
type SubtypeDecl   struct{ P Pos; Name string; SubtypeMark string; Constraint Expr }
type TypeDecl      struct{ P Pos; Name string; Def TypeDef }
type ComponentDecl struct{ P Pos; Name string; Generics []*InterfaceDecl; Ports []*InterfaceDecl }
type InterfaceDecl struct{ P Pos; ObjClass string; Names []string; Mode string; SubtypeMark string; Constraint Expr; Default Expr } // ObjClass: "" | "constant" | "signal" | "variable"; Mode: "" | "in" | "out" | "inout" | "buffer"

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

// SubprogramBody is a function/procedure body: spec `is <decls> begin <stmts> end ;`.
type SubprogramBody struct {
	P           Pos
	IsProcedure bool
	Pure        bool
	Impure      bool
	Designator  string
	Params      []*InterfaceDecl
	ReturnMark  string
	Decls       []Decl
	Stmts       []Stmt
}

func (n *SubprogramBody) Pos() Pos { return n.P }
func (n *SubprogramBody) End() Pos {
	if k := len(n.Stmts); k > 0 { return n.Stmts[k-1].End() }
	if k := len(n.Decls); k > 0 { return n.Decls[k-1].End() }
	return n.P
}
func (n *SubprogramBody) declNode() {}

// Signature is an alias/attribute subprogram signature:
// `[ [type_mark {, type_mark}] [return type_mark] ]`.
type Signature struct {
	Types  []string // parameter/profile type marks
	Return string   // return type mark ("" if none)
}

// AliasDecl is `alias name [: subtype_indication] is target [signature] ;`. SubtypeMark is ""
// when no subtype indication is present. Target is the aliased name expression.
type AliasDecl struct {
	P           Pos
	Name        string
	SubtypeMark string
	Constraint  Expr
	Target      Expr
	Signature   *Signature
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

// FileDecl is `file names : subtype_mark [ [open expr] is expr ] ;`.
type FileDecl struct {
	P           Pos
	Names       []string
	SubtypeMark string
	Mode        string // VHDL-87 file mode: "" | "in" | "out"
	OpenMode    Expr   // file_open_kind expression (nil if absent)
	LogicalName Expr   // file_logical_name expression (nil if absent)
}

func (n *FileDecl) Pos() Pos { return n.P }
func (n *FileDecl) End() Pos {
	if n.LogicalName != nil { return n.LogicalName.End() }
	if n.OpenMode != nil { return n.OpenMode.End() }
	return n.P
}
func (n *FileDecl) declNode() {}

// FileTypeDef is `file of type_mark`.
type FileTypeDef struct{ P Pos; Mark string }

func (n *FileTypeDef) Pos() Pos { return n.P }
func (n *FileTypeDef) End() Pos { return n.P }
func (n *FileTypeDef) typeDefNode() {}

// AccessDef is `access subtype_mark`.
type AccessDef struct{ P Pos; Mark string }

func (n *AccessDef) Pos() Pos { return n.P }
func (n *AccessDef) End() Pos { return n.P }
func (n *AccessDef) typeDefNode() {}

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

// PhysicalLit is an abstract literal with a unit name, e.g. `5 ns`.
type PhysicalLit struct {
	ValuePos Pos
	Value    string // the abstract literal text (e.g. "5", "1.5", "16#FF#")
	Unit     string // the unit name (e.g. "ns")
}

func (n *PhysicalLit) Pos() Pos { return n.ValuePos }
func (n *PhysicalLit) End() Pos { return n.ValuePos + Pos(len(n.Value)+1+len(n.Unit)) }
func (n *PhysicalLit) exprNode() {}

// expressions
type BasicLit    struct{ ValuePos Pos; Kind Kind; Value string } // INT/REAL/BASEDLIT/CHARLIT/STRINGLIT/BITSTRINGLIT
type Ident       struct{ NamePos Pos; Name string }              // a (possibly compound/attributed) name; full decomposition into SelectorExpr is deferred
type Range       struct{ Left Expr; DirPos Pos; Dir Kind; Right Expr } // Dir is TO or DOWNTO
type CallExpr     struct{ Fun Expr; Lparen Pos; Args []*AssocElement; Rparen Pos } // also indexed-name / slice / type-conversion — VHDL can't disambiguate syntactically; positional args have Formal ""
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

// SelectorExpr is a selected name `X.Sel` where X is a non-simple prefix (e.g. a
// call/indexed result). A leading run of simple dotted names is flattened into
// Ident.Name instead; SelectorExpr is produced only for a selection after a
// call/index suffix.
type SelectorExpr struct {
	X   Expr
	Dot Pos
	Sel string
}

func (n *SelectorExpr) Pos() Pos { return n.X.Pos() }
func (n *SelectorExpr) End() Pos { return n.Dot + Pos(len(n.Sel)+1) }
func (n *SelectorExpr) exprNode() {}

// AttributeName is an attribute applied to a prefix expression, e.g.
// dr_tmp(i)'LAST_EVENT or arr(i)'length. (Attributes on a SIMPLE name are
// folded into Ident.Name by the parser; this node carries the non-flat case.)
type AttributeName struct {
	X    Expr
	Tick Pos
	Attr string
}

func (a *AttributeName) Pos() Pos { return a.X.Pos() }
func (a *AttributeName) End() Pos { return a.Tick + Pos(len(a.Attr)+1) }
func (*AttributeName) exprNode()  {}

func (n *BasicLit)   exprNode() {}
func (n *Ident)      exprNode() {}
func (n *Range)      exprNode() {}
func (n *CallExpr)   exprNode() {}
func (n *BinaryExpr) exprNode() {}
func (n *UnaryExpr)  exprNode() {}
func (n *ParenExpr)  exprNode() {}

// RangeConstraint is a discrete range with a type mark: `integer range 0 to 7`.
type RangeConstraint struct {
	P     Pos
	Mark  Expr // the type mark
	Range Expr // the range after `range`
}

func (r *RangeConstraint) Pos() Pos    { return r.P }
func (r *RangeConstraint) End() Pos    { return r.Range.End() }
func (*RangeConstraint)   exprNode()   {}

// AllocatorExpr is a `new <subtype_indication | qualified_expression>` allocator.
type AllocatorExpr struct {
	New Pos
	X   Expr
}

func (a *AllocatorExpr) Pos() Pos    { return a.New }
func (a *AllocatorExpr) End() Pos    { return a.X.End() }
func (*AllocatorExpr)   exprNode()   {}
