package vhdl

import (
	"reflect"
	"strings"
)

// indentUnit is one level of VHDL indentation. The vmagic/soc_gen golden uses 4
// spaces per level.
const indentUnit = "    "

// Print renders a DesignFile back to canonical VHDL text.
func Print(f *DesignFile) string {
	var b strings.Builder
	printFile(&b, f)
	return b.String()
}

// SubtypeString renders a subtype indication (type mark + optional constraint)
// to canonical VHDL, e.g. "std_logic_vector(15 downto 0)" or "integer range 0 to 7".
func SubtypeString(mark string, constraint Expr) string {
	var b strings.Builder
	printSubtypeIndication(&b, mark, constraint)
	return b.String()
}

func printFile(b *strings.Builder, f *DesignFile) {
	for _, c := range f.Context {
		switch n := c.(type) {
		case *LibraryClause:
			b.WriteString("library ")
			b.WriteString(strings.Join(n.Names, ", "))
			b.WriteString(";\n")
		case *UseClause:
			b.WriteString("use ")
			b.WriteString(strings.Join(n.Names, ", "))
			b.WriteString(";\n")
		}
	}
	for _, u := range f.Units {
		printUnit(b, u)
	}
}

func printUnit(b *strings.Builder, u DesignUnit) {
	switch n := u.(type) {
	case *PackageDecl:
		printPackageDecl(b, n)
	case *PackageBody:
		printPackageBody(b, n)
	case *EntityDecl:
		printEntityDecl(b, n)
	case *ArchitectureBody:
		printArchitectureBody(b, n)
	case *ConfigurationDecl:
		printConfigurationDecl(b, n)
	}
}

func printConfigurationDecl(b *strings.Builder, n *ConfigurationDecl) {
	b.WriteString("configuration ")
	b.WriteString(n.Name)
	b.WriteString(" of ")
	b.WriteString(n.Entity)
	b.WriteString(" is\n")
	for _, u := range n.Decls {
		b.WriteString(indentUnit + "use ")
		b.WriteString(strings.Join(u.Names, ", "))
		b.WriteString(";\n")
	}
	if n.Block != nil {
		printBlockConfig(b, n.Block, indentUnit)
	}
	b.WriteString("end configuration;\n")
}

func printBlockConfig(b *strings.Builder, n *BlockConfig, indent string) {
	b.WriteString(indent)
	b.WriteString("for ")
	b.WriteString(n.Spec)
	b.WriteByte('\n')
	for _, u := range n.Uses {
		b.WriteString(indent)
		b.WriteString(indentUnit + "use ")
		b.WriteString(strings.Join(u.Names, ", "))
		b.WriteString(";\n")
	}
	for _, it := range n.Items {
		switch c := it.(type) {
		case *BlockConfig:
			printBlockConfig(b, c, indent+indentUnit)
		case *ComponentConfig:
			printComponentConfig(b, c, indent+indentUnit)
		}
	}
	b.WriteString(indent)
	b.WriteString("end for;\n")
}

func printComponentConfig(b *strings.Builder, n *ComponentConfig, indent string) {
	b.WriteString(indent)
	b.WriteString("for ")
	b.WriteString(strings.Join(n.Insts, ", "))
	b.WriteString(" : ")
	b.WriteString(n.Comp)
	b.WriteByte('\n')
	if n.Binding != nil {
		b.WriteString(indent)
		b.WriteString(indentUnit)
		printBindingIndication(b, n.Binding)
		b.WriteString(";\n")
	}
	if n.Block != nil {
		printBlockConfig(b, n.Block, indent+indentUnit)
	}
	b.WriteString(indent)
	b.WriteString("end for;\n")
}

func printBindingIndication(b *strings.Builder, n *BindingIndication) {
	b.WriteString("use ")
	switch n.UnitKind {
	case ENTITY:
		b.WriteString("entity ")
		b.WriteString(n.Unit)
		if n.Arch != "" {
			b.WriteByte('(')
			b.WriteString(n.Arch)
			b.WriteByte(')')
		}
	case CONFIGURATION:
		b.WriteString("configuration ")
		b.WriteString(n.Unit)
	case OPEN:
		b.WriteString("open")
	}
	if len(n.GenericMap) > 0 {
		b.WriteString(" generic map (")
		printAssocList(b, n.GenericMap)
		b.WriteByte(')')
	}
	if len(n.PortMap) > 0 {
		b.WriteString(" port map (")
		printAssocList(b, n.PortMap)
		b.WriteByte(')')
	}
}

func printArchitectureBody(b *strings.Builder, n *ArchitectureBody) {
	b.WriteString("architecture ")
	b.WriteString(n.Name)
	b.WriteString(" of ")
	b.WriteString(n.Entity)
	b.WriteString(" is\n")
	for _, d := range n.Decls {
		b.WriteString(indentUnit)
		printDecl(b, d, indentUnit)
		b.WriteByte('\n')
	}
	b.WriteString("begin\n")
	for _, s := range n.Stmts {
		b.WriteString(indentUnit)
		printStmt(b, s, indentUnit)
		b.WriteByte('\n')
	}
	b.WriteString("end;\n")
}

// printStmt prints a concurrent (or, later, sequential) statement.
func printStmt(b *strings.Builder, s Stmt, indent string) {
	switch n := s.(type) {
	case *ConcurrentSignalAssign:
		if n.Label != "" {
			b.WriteString(n.Label)
			b.WriteString(" : ")
		}
		printExpr(b, n.Target)
		b.WriteString(" <= ")
		printDelay(b, n.Delay)
		if len(n.Waveform) > 0 {
			printWaveform(b, n.Waveform)
		} else {
			for i, c := range n.Conds {
				if i > 0 {
					b.WriteString(" else ")
				}
				printWaveform(b, c.Waveform)
				if c.Cond != nil {
					b.WriteString(" when ")
					printExpr(b, c.Cond)
				}
			}
		}
		b.WriteByte(';')
	case *InstantiationStmt:
		if n.Label != "" {
			b.WriteString(n.Label)
			b.WriteString(" : ")
		}
		switch n.UnitKind {
		case ENTITY:
			b.WriteString("entity ")
		case COMPONENT:
			b.WriteString("component ")
		case CONFIGURATION:
			b.WriteString("configuration ")
		}
		b.WriteString(n.Unit)
		if n.Arch != "" {
			b.WriteByte('(')
			b.WriteString(n.Arch)
			b.WriteByte(')')
		}
		mapIndent := indent + indentUnit
		if len(n.GenericMap) > 0 {
			b.WriteByte('\n')
			printInstMap(b, mapIndent, "generic map", n.GenericMap)
		}
		if len(n.PortMap) > 0 {
			b.WriteByte('\n')
			printInstMap(b, mapIndent, "port map", n.PortMap)
		}
		b.WriteByte(';')
	case *GenerateStmt:
		if n.Label != "" {
			b.WriteString(n.Label)
			b.WriteString(" : ")
		}
		if n.Kind == FOR {
			b.WriteString("for ")
			b.WriteString(n.Param)
			b.WriteString(" in ")
			printExpr(b, n.Range)
		} else {
			b.WriteString("if ")
			printExpr(b, n.Cond)
		}
		b.WriteString(" generate\n")
		for _, d := range n.Decls {
			b.WriteString(indent)
			b.WriteString(indentUnit)
			printDecl(b, d, indent+indentUnit)
			b.WriteByte('\n')
		}
		if len(n.Decls) > 0 {
			b.WriteString(indent)
			b.WriteString("begin\n")
		}
		for _, s := range n.Stmts {
			b.WriteString(indent)
			b.WriteString(indentUnit)
			printStmt(b, s, indent+indentUnit)
			b.WriteByte('\n')
		}
		b.WriteString(indent)
		b.WriteString("end generate;")
	case *ProcessStmt:
		if n.Label != "" {
			b.WriteString(n.Label)
			b.WriteString(" : ")
		}
		if n.Postponed {
			b.WriteString("postponed ")
		}
		b.WriteString("process")
		if len(n.Sensitivity) > 0 {
			b.WriteString(" (")
			for i, s := range n.Sensitivity {
				if i > 0 {
					b.WriteString(", ")
				}
				printExpr(b, s)
			}
			b.WriteByte(')')
		}
		b.WriteByte('\n')
		for _, d := range n.Decls {
			b.WriteString(indent)
			b.WriteString(indentUnit)
			printDecl(b, d, indent+indentUnit)
			b.WriteByte('\n')
		}
		b.WriteString(indent)
		b.WriteString("begin\n")
		for _, s := range n.Stmts {
			b.WriteString(indent)
			b.WriteString(indentUnit)
			printStmt(b, s, indent+indentUnit)
			b.WriteByte('\n')
		}
		b.WriteString(indent)
		b.WriteString("end process;")
	case *SignalAssignStmt:
		if n.Label != "" {
			b.WriteString(n.Label)
			b.WriteString(" : ")
		}
		printExpr(b, n.Target)
		b.WriteString(" <= ")
		printDelay(b, n.Delay)
		printWaveform(b, n.Waveform)
		b.WriteByte(';')
	case *VariableAssignStmt:
		if n.Label != "" {
			b.WriteString(n.Label)
			b.WriteString(" : ")
		}
		printExpr(b, n.Target)
		b.WriteString(" := ")
		printExpr(b, n.Value)
		b.WriteByte(';')
	case *NullStmt:
		if n.Label != "" {
			b.WriteString(n.Label)
			b.WriteString(" : ")
		}
		b.WriteString("null;")
	case *IfStmt:
		if n.Label != "" {
			b.WriteString(n.Label)
			b.WriteString(" : ")
		}
		b.WriteString("if ")
		printExpr(b, n.Cond)
		b.WriteString(" then\n")
		printSeqStmts(b, n.Then, indent)
		for _, ei := range n.Elsifs {
			b.WriteString(indent)
			b.WriteString("elsif ")
			printExpr(b, ei.Cond)
			b.WriteString(" then\n")
			printSeqStmts(b, ei.Stmts, indent)
		}
		if n.Else != nil {
			b.WriteString(indent)
			b.WriteString("else\n")
			printSeqStmts(b, n.Else, indent)
		}
		b.WriteString(indent)
		b.WriteString("end if;")
	case *CaseStmt:
		if n.Label != "" {
			b.WriteString(n.Label)
			b.WriteString(" : ")
		}
		b.WriteString("case ")
		printExpr(b, n.Expr)
		b.WriteString(" is\n")
		for _, alt := range n.Alts {
			b.WriteString(indent)
			b.WriteString(indentUnit + "when ")
			for i, c := range alt.Choices {
				if i > 0 {
					b.WriteString(" | ")
				}
				printExpr(b, c)
			}
			b.WriteString(" =>\n")
			printSeqStmts(b, alt.Stmts, indent+indentUnit)
		}
		b.WriteString(indent)
		b.WriteString("end case;")
	case *LoopStmt:
		if n.Label != "" {
			b.WriteString(n.Label)
			b.WriteString(" : ")
		}
		switch n.Scheme {
		case FOR:
			b.WriteString("for ")
			b.WriteString(n.Param)
			b.WriteString(" in ")
			printExpr(b, n.Range)
			b.WriteByte(' ')
		case WHILE:
			b.WriteString("while ")
			printExpr(b, n.Cond)
			b.WriteByte(' ')
		}
		b.WriteString("loop\n")
		printSeqStmts(b, n.Stmts, indent)
		b.WriteString(indent)
		b.WriteString("end loop;")
	case *NextStmt:
		if n.Label != "" {
			b.WriteString(n.Label)
			b.WriteString(" : ")
		}
		b.WriteString("next")
		if n.LoopLabel != "" {
			b.WriteByte(' ')
			b.WriteString(n.LoopLabel)
		}
		if n.When != nil {
			b.WriteString(" when ")
			printExpr(b, n.When)
		}
		b.WriteByte(';')
	case *ExitStmt:
		if n.Label != "" {
			b.WriteString(n.Label)
			b.WriteString(" : ")
		}
		b.WriteString("exit")
		if n.LoopLabel != "" {
			b.WriteByte(' ')
			b.WriteString(n.LoopLabel)
		}
		if n.When != nil {
			b.WriteString(" when ")
			printExpr(b, n.When)
		}
		b.WriteByte(';')
	case *ReturnStmt:
		if n.Label != "" {
			b.WriteString(n.Label)
			b.WriteString(" : ")
		}
		b.WriteString("return")
		if n.Value != nil {
			b.WriteByte(' ')
			printExpr(b, n.Value)
		}
		b.WriteByte(';')
	case *AssertStmt:
		if n.Label != "" {
			b.WriteString(n.Label)
			b.WriteString(" : ")
		}
		b.WriteString("assert ")
		printExpr(b, n.Cond)
		if n.Report != nil {
			b.WriteString(" report ")
			printExpr(b, n.Report)
		}
		if n.Severity != nil {
			b.WriteString(" severity ")
			printExpr(b, n.Severity)
		}
		b.WriteByte(';')
	case *ReportStmt:
		if n.Label != "" {
			b.WriteString(n.Label)
			b.WriteString(" : ")
		}
		b.WriteString("report ")
		printExpr(b, n.Report)
		if n.Severity != nil {
			b.WriteString(" severity ")
			printExpr(b, n.Severity)
		}
		b.WriteByte(';')
	case *WaitStmt:
		if n.Label != "" {
			b.WriteString(n.Label)
			b.WriteString(" : ")
		}
		b.WriteString("wait")
		if len(n.On) > 0 {
			b.WriteString(" on ")
			for i, s := range n.On {
				if i > 0 {
					b.WriteString(", ")
				}
				printExpr(b, s)
			}
		}
		if n.Until != nil {
			b.WriteString(" until ")
			printExpr(b, n.Until)
		}
		if n.For != nil {
			b.WriteString(" for ")
			printExpr(b, n.For)
		}
		b.WriteByte(';')

	case *ProcedureCallStmt:
		if n.Label != "" {
			b.WriteString(n.Label)
			b.WriteString(" : ")
		}
		b.WriteString(n.Name)
		if len(n.Args) > 0 {
			b.WriteByte('(')
			printAssocList(b, n.Args)
			b.WriteByte(')')
		}
		b.WriteByte(';')
	case *SelectedSignalAssign:
		if n.Label != "" {
			b.WriteString(n.Label)
			b.WriteString(" : ")
		}
		b.WriteString("with ")
		printExpr(b, n.Expr)
		b.WriteString(" select ")
		printExpr(b, n.Target)
		b.WriteString(" <= ")
		printDelay(b, n.Delay)
		for i, alt := range n.Alts {
			if i > 0 {
				b.WriteString(", ")
			}
			printWaveform(b, alt.Waveform)
			b.WriteString(" when ")
			for j, c := range alt.Choices {
				if j > 0 {
					b.WriteString(" | ")
				}
				printExpr(b, c)
			}
		}
		b.WriteByte(';')
	}
}

// printSeqStmts prints a sequential statement list, each indented under indent.
func printSeqStmts(b *strings.Builder, stmts []Stmt, indent string) {
	for _, s := range stmts {
		b.WriteString(indent)
		b.WriteString(indentUnit)
		printStmt(b, s, indent+indentUnit)
		b.WriteByte('\n')
	}
}

// printDelay prints an optional delay mechanism before a waveform.
func printDelay(b *strings.Builder, d *DelayMechanism) {
	if d == nil {
		return
	}
	if d.Transport {
		b.WriteString("transport ")
		return
	}
	if d.Reject != nil {
		b.WriteString("reject ")
		printExpr(b, d.Reject)
		b.WriteString(" inertial ")
		return
	}
	b.WriteString("inertial ")
}

// printWaveform prints `value [after time]{, value after time}`.
func printWaveform(b *strings.Builder, wf []*WaveformElem) {
	for i, el := range wf {
		if i > 0 {
			b.WriteString(", ")
		}
		printExpr(b, el.Value)
		if el.After != nil {
			b.WriteString(" after ")
			printExpr(b, el.After)
		}
	}
}

func printAssocList(b *strings.Builder, elems []*AssocElement) {
	for i, e := range elems {
		if i > 0 {
			b.WriteString(", ")
		}
		if e.Formal != "" {
			b.WriteString(e.Formal)
			b.WriteString(" => ")
		}
		printExpr(b, e.Actual)
	}
}

// printInstMap prints a multi-line generic-map/port-map association list:
//
//	<mapIndent><keyword> (
//	<mapIndent><indentUnit><formal> => <actual>,
//	...
//	<mapIndent>)
//
// Association ORDER IS PRESERVED — emit sorts the AST; the printer must not
// reorder, or the order-sensitive corpus round-trip (equalAST) would fail. The
// last association has no trailing comma. The caller writes the preceding newline.
func printInstMap(b *strings.Builder, mapIndent, keyword string, elems []*AssocElement) {
	b.WriteString(mapIndent)
	b.WriteString(keyword)
	b.WriteString(" (\n")
	for i, e := range elems {
		b.WriteString(mapIndent)
		b.WriteString(indentUnit)
		if e.Formal != "" {
			b.WriteString(e.Formal)
			b.WriteString(" => ")
		}
		printExpr(b, e.Actual)
		if i < len(elems)-1 {
			b.WriteByte(',')
		}
		b.WriteByte('\n')
	}
	b.WriteString(mapIndent)
	b.WriteByte(')')
}

// exprString renders an expression to its canonical text (used for association
// formals, which are stored as strings).
func exprString(e Expr) string {
	var b strings.Builder
	printExpr(&b, e)
	return b.String()
}

func printPackageDecl(b *strings.Builder, n *PackageDecl) {
	b.WriteString("package ")
	b.WriteString(n.Name)
	b.WriteString(" is\n")
	for _, d := range n.Decls {
		b.WriteString(indentUnit)
		printDecl(b, d, indentUnit)
		b.WriteByte('\n')
	}
	b.WriteString("end package;\n")
}

func printPackageBody(b *strings.Builder, n *PackageBody) {
	b.WriteString("package body ")
	b.WriteString(n.Name)
	b.WriteString(" is\n")
	for _, d := range n.Decls {
		b.WriteString(indentUnit)
		printDecl(b, d, indentUnit)
		b.WriteByte('\n')
	}
	b.WriteString("end package body;\n")
}

func printEntityDecl(b *strings.Builder, n *EntityDecl) {
	b.WriteString("entity ")
	b.WriteString(n.Name)
	b.WriteString(" is\n")
	if len(n.Generics) > 0 {
		b.WriteString(indentUnit + "generic (\n")
		printInterfaceList(b, n.Generics, indentUnit+indentUnit)
		b.WriteString(indentUnit + ");\n")
	}
	if len(n.Ports) > 0 {
		b.WriteString(indentUnit + "port (\n")
		printInterfaceList(b, n.Ports, indentUnit+indentUnit)
		b.WriteString(indentUnit + ");\n")
	}
	for _, d := range n.Decls {
		b.WriteString(indentUnit)
		printDecl(b, d, indentUnit)
		b.WriteByte('\n')
	}
	if len(n.Stmts) > 0 {
		b.WriteString("begin\n")
		for _, s := range n.Stmts {
			b.WriteString(indentUnit)
			printStmt(b, s, indentUnit)
			b.WriteByte('\n')
		}
	}
	b.WriteString("end;\n")
}

func printInterfaceList(b *strings.Builder, decls []*InterfaceDecl, indent string) {
	for i, d := range decls {
		b.WriteString(indent)
		printInterfaceDecl(b, d)
		if i < len(decls)-1 {
			b.WriteString(";\n")
		} else {
			b.WriteByte('\n')
		}
	}
}

func printInterfaceDecl(b *strings.Builder, d *InterfaceDecl) {
	if d.ObjClass != "" {
		b.WriteString(d.ObjClass)
		b.WriteByte(' ')
	}
	b.WriteString(strings.Join(d.Names, ", "))
	b.WriteString(" : ")
	if d.Mode != "" {
		b.WriteString(d.Mode)
		b.WriteByte(' ')
	}
	printSubtypeIndication(b, d.SubtypeMark, d.Constraint)
	if d.Default != nil {
		b.WriteString(" := ")
		printExpr(b, d.Default)
	}
}

func printDecl(b *strings.Builder, d Decl, indent string) {
	switch n := d.(type) {
	case *ConstantDecl:
		b.WriteString("constant ")
		b.WriteString(strings.Join(n.Names, ", "))
		b.WriteString(" : ")
		printSubtypeIndication(b, n.SubtypeMark, n.Constraint)
		if n.Default != nil {
			b.WriteString(" := ")
			printExpr(b, n.Default)
		}
		b.WriteByte(';')
	case *SignalDecl:
		b.WriteString("signal ")
		b.WriteString(strings.Join(n.Names, ", "))
		b.WriteString(" : ")
		printSubtypeIndication(b, n.SubtypeMark, n.Constraint)
		if n.Default != nil {
			b.WriteString(" := ")
			printExpr(b, n.Default)
		}
		b.WriteByte(';')
	case *VariableDecl:
		if n.Shared {
			b.WriteString("shared ")
		}
		b.WriteString("variable ")
		b.WriteString(strings.Join(n.Names, ", "))
		b.WriteString(" : ")
		printSubtypeIndication(b, n.SubtypeMark, n.Constraint)
		if n.Default != nil {
			b.WriteString(" := ")
			printExpr(b, n.Default)
		}
		b.WriteByte(';')
	case *SubtypeDecl:
		b.WriteString("subtype ")
		b.WriteString(n.Name)
		b.WriteString(" is ")
		printSubtypeIndication(b, n.SubtypeMark, n.Constraint)
		b.WriteByte(';')
	case *TypeDecl:
		printTypeDecl(b, n, indent)
	case *ComponentDecl:
		printComponentDecl(b, n, indent)
	case *AttributeDecl:
		b.WriteString("attribute ")
		b.WriteString(n.Name)
		b.WriteString(" : ")
		b.WriteString(n.TypeMark)
		b.WriteByte(';')
	case *AttributeSpec:
		b.WriteString("attribute ")
		b.WriteString(n.Name)
		b.WriteString(" of ")
		b.WriteString(strings.Join(n.Entities, ", "))
		b.WriteString(" : ")
		b.WriteString(n.EntityClass.String())
		b.WriteString(" is ")
		printExpr(b, n.Value)
		b.WriteByte(';')
	case *AliasDecl:
		b.WriteString("alias ")
		b.WriteString(n.Name)
		if n.SubtypeMark != "" {
			b.WriteString(" : ")
			printSubtypeIndication(b, n.SubtypeMark, n.Constraint)
		}
		b.WriteString(" is ")
		printExpr(b, n.Target)
		if n.Signature != nil {
			b.WriteString(" [")
			b.WriteString(strings.Join(n.Signature.Types, ", "))
			if n.Signature.Return != "" {
				if len(n.Signature.Types) > 0 {
					b.WriteByte(' ')
				}
				b.WriteString("return ")
				b.WriteString(n.Signature.Return)
			}
			b.WriteByte(']')
		}
		b.WriteByte(';')
	case *GroupTemplateDecl:
		b.WriteString("group ")
		b.WriteString(n.Name)
		b.WriteString(" is (")
		b.WriteString(strings.Join(n.Classes, ", "))
		b.WriteString(");")
	case *GroupDecl:
		b.WriteString("group ")
		b.WriteString(n.Name)
		b.WriteString(" : ")
		b.WriteString(n.TemplateMark)
		b.WriteByte('(')
		b.WriteString(strings.Join(n.Constituents, ", "))
		b.WriteString(");")
	case *ConfigSpec:
		b.WriteString("for ")
		b.WriteString(strings.Join(n.Insts, ", "))
		b.WriteString(" : ")
		b.WriteString(n.Comp)
		b.WriteByte(' ')
		if n.Binding != nil {
			printBindingIndication(b, n.Binding)
		}
		b.WriteByte(';')
	case *FileDecl:
		b.WriteString("file ")
		b.WriteString(strings.Join(n.Names, ", "))
		b.WriteString(" : ")
		b.WriteString(n.SubtypeMark)
		if n.OpenMode != nil {
			b.WriteString(" open ")
			printExpr(b, n.OpenMode)
			b.WriteString(" is ")
			printExpr(b, n.LogicalName)
		} else if n.LogicalName != nil {
			b.WriteString(" is ")
			if n.Mode != "" {
				b.WriteString(n.Mode)
				b.WriteByte(' ')
			}
			printExpr(b, n.LogicalName)
		}
		b.WriteByte(';')
	case *UseClause:
		b.WriteString("use ")
		b.WriteString(strings.Join(n.Names, ", "))
		b.WriteByte(';')
	case *SubprogramDecl:
		printSubprogramSpec(b, n.IsProcedure, n.Pure, n.Impure, n.Designator, n.Params, n.ReturnMark)
		b.WriteByte(';')
	case *SubprogramBody:
		printSubprogramSpec(b, n.IsProcedure, n.Pure, n.Impure, n.Designator, n.Params, n.ReturnMark)
		b.WriteString(" is\n")
		for _, d := range n.Decls {
			b.WriteString(indent)
			b.WriteString(indentUnit)
			printDecl(b, d, indent+indentUnit)
			b.WriteByte('\n')
		}
		b.WriteString(indent)
		b.WriteString("begin\n")
		printSeqStmts(b, n.Stmts, indent)
		b.WriteString(indent)
		if n.IsProcedure {
			b.WriteString("end procedure;")
		} else {
			b.WriteString("end;")
		}
	}
}

// printSubprogramSpec prints `[pure|impure] function|procedure desig[(params)][ return mark]`.
func printSubprogramSpec(b *strings.Builder, isProc, pure, impure bool, desig string, params []*InterfaceDecl, ret string) {
	if pure {
		b.WriteString("pure ")
	} else if impure {
		b.WriteString("impure ")
	}
	if isProc {
		b.WriteString("procedure ")
	} else {
		b.WriteString("function ")
	}
	b.WriteString(desig)
	if len(params) > 0 {
		b.WriteByte('(')
		for i, prm := range params {
			if i > 0 {
				b.WriteString("; ")
			}
			printInterfaceDecl(b, prm)
		}
		b.WriteByte(')')
	}
	if !isProc {
		b.WriteString(" return ")
		b.WriteString(ret)
	}
}

func printTypeDecl(b *strings.Builder, n *TypeDecl, indent string) {
	b.WriteString("type ")
	b.WriteString(n.Name)
	b.WriteString(" is ")
	switch def := n.Def.(type) {
	case *EnumDef:
		b.WriteByte('(')
		b.WriteString(strings.Join(def.Lits, ", "))
		b.WriteString(");")
	case *RecordDef:
		b.WriteString("record\n")
		for _, f := range def.Fields {
			b.WriteString(indent)
			b.WriteString(indentUnit)
			b.WriteString(strings.Join(f.Names, ", "))
			b.WriteString(" : ")
			printSubtypeIndication(b, f.SubtypeMark, f.Constraint)
			b.WriteString(";\n")
		}
		b.WriteString(indent)
		b.WriteString("end record;")
	case *ArrayDef:
		b.WriteString(def.Text)
		b.WriteByte(';')
	case *FileTypeDef:
		b.WriteString("file of ")
		b.WriteString(def.Mark)
		b.WriteByte(';')
	case *AccessDef:
		b.WriteString("access ")
		b.WriteString(def.Mark)
		b.WriteByte(';')
	}
}

func printComponentDecl(b *strings.Builder, n *ComponentDecl, indent string) {
	b.WriteString("component ")
	b.WriteString(n.Name)
	b.WriteString(" is\n")
	if len(n.Generics) > 0 {
		b.WriteString(indent)
		b.WriteString(indentUnit + "generic (\n")
		printInterfaceList(b, n.Generics, indent+indentUnit+indentUnit)
		b.WriteString(indent)
		b.WriteString(indentUnit + ");\n")
	}
	if len(n.Ports) > 0 {
		b.WriteString(indent)
		b.WriteString(indentUnit + "port (\n")
		printInterfaceList(b, n.Ports, indent+indentUnit+indentUnit)
		b.WriteString(indent)
		b.WriteString(indentUnit + ");\n")
	}
	b.WriteString(indent)
	b.WriteString("end component;")
}

// printSubtypeIndication prints "mark", "mark(constraint)", or
// "mark range <expr>" canonically. A ParenExpr or Aggregate constraint emits its
// OWN surrounding parentheses, so we must not add a second pair here.
func printSubtypeIndication(b *strings.Builder, mark string, constraint Expr) {
	b.WriteString(mark)
	switch c := constraint.(type) {
	case nil:
		// no constraint
	case *ParenExpr, *Aggregate:
		printExpr(b, c) // c prints its own surrounding parentheses: mark(...)
	default:
		// a `range` constraint: `mark range <expr>` (parseSubtypeIndication's
		// RANGE case stores a bare expr here)
		b.WriteString(" range ")
		printExpr(b, c)
	}
}

func printExpr(b *strings.Builder, e Expr) {
	switch n := e.(type) {
	case *PhysicalLit:
		b.WriteString(n.Value)
		b.WriteByte(' ')
		b.WriteString(n.Unit)
	case *BasicLit:
		b.WriteString(n.Value)
	case *Ident:
		b.WriteString(n.Name)
	case *Range:
		printExpr(b, n.Left)
		b.WriteByte(' ')
		b.WriteString(n.Dir.String())
		b.WriteByte(' ')
		printExpr(b, n.Right)
	case *BinaryExpr:
		printExpr(b, n.X)
		b.WriteByte(' ')
		b.WriteString(n.Op.String())
		b.WriteByte(' ')
		printExpr(b, n.Y)
	case *UnaryExpr:
		// Always emit a space after the unary operator. This is mandatory for
		// "-"/"+": it avoids "- -a" printing as "--a" (a comment to the lexer)
		// and "**"-adjacency hazards. Reparsing "- a" yields the same tree as
		// "-a", so round-trip equality holds.
		b.WriteString(n.Op.String())
		b.WriteByte(' ')
		printExpr(b, n.X)
	case *CallExpr:
		printExpr(b, n.Fun)
		b.WriteByte('(')
		printAssocList(b, n.Args)
		b.WriteByte(')')
	case *SelectorExpr:
		printExpr(b, n.X)
		b.WriteByte('.')
		b.WriteString(n.Sel)
	case *AttributeName:
		printExpr(b, n.X)
		b.WriteByte('\'')
		b.WriteString(n.Attr)
	case *ParenExpr:
		b.WriteByte('(')
		printExpr(b, n.X)
		b.WriteByte(')')
	case *Aggregate:
		b.WriteByte('(')
		for i, e := range n.Elems {
			if i > 0 {
				b.WriteString(", ")
			}
			printElementAssoc(b, e)
		}
		b.WriteByte(')')
	case *QualifiedExpr:
		printExpr(b, n.Mark)
		b.WriteByte('\'')
		printExpr(b, n.X) // X is a ParenExpr/Aggregate that prints its own parens
	case *RangeConstraint:
		printExpr(b, n.Mark)
		b.WriteString(" range ")
		printExpr(b, n.Range)
	case *AllocatorExpr:
		b.WriteString("new ")
		printExpr(b, n.X)
	}
}

func printElementAssoc(b *strings.Builder, e *ElementAssoc) {
	for i, c := range e.Choices {
		if i > 0 {
			b.WriteString(" | ")
		}
		printExpr(b, c)
	}
	if e.Choices != nil {
		b.WriteString(" => ")
	}
	printExpr(b, e.X)
}

// equalAST reports structural equality between two AST nodes, ignoring Pos fields.
func equalAST(a, b Node) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return eq(reflect.ValueOf(a), reflect.ValueOf(b))
}

// eq recursively compares two reflect.Values for structural equality,
// ignoring any field of type Pos.
func eq(x, y reflect.Value) bool {
	// Dereference interface wrappers first.
	if x.Kind() == reflect.Interface {
		if x.IsNil() && y.IsNil() {
			return true
		}
		if x.IsNil() || y.IsNil() {
			return false
		}
		return eq(x.Elem(), y.Elem())
	}

	// Types must match.
	if x.Type() != y.Type() {
		return false
	}

	// Ignore Pos fields entirely.
	if x.Type() == reflect.TypeFor[Pos]() {
		return true
	}

	switch x.Kind() {
	case reflect.Ptr:
		if x.IsNil() && y.IsNil() {
			return true
		}
		if x.IsNil() || y.IsNil() {
			return false
		}
		return eq(x.Elem(), y.Elem())

	case reflect.Struct:
		for i := 0; i < x.NumField(); i++ {
			if !eq(x.Field(i), y.Field(i)) {
				return false
			}
		}
		return true

	case reflect.Slice:
		if x.IsNil() != y.IsNil() {
			// Treat nil and empty slice as equal for AST comparison purposes.
			if x.Len() == 0 && y.Len() == 0 {
				return true
			}
		}
		if x.Len() != y.Len() {
			return false
		}
		for i := 0; i < x.Len(); i++ {
			if !eq(x.Index(i), y.Index(i)) {
				return false
			}
		}
		return true

	case reflect.String:
		return x.String() == y.String()

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return x.Int() == y.Int()

	case reflect.Bool:
		return x.Bool() == y.Bool()

	default:
		// Fallback to DeepEqual for any other primitive kinds.
		return reflect.DeepEqual(x.Interface(), y.Interface())
	}
}
