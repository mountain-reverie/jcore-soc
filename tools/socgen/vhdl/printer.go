package vhdl

import (
	"reflect"
	"strings"
)

// Print renders a DesignFile back to canonical VHDL text.
func Print(f *DesignFile) string {
	var b strings.Builder
	printFile(&b, f)
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
	case *EntityDecl:
		printEntityDecl(b, n)
	}
}

func printPackageDecl(b *strings.Builder, n *PackageDecl) {
	b.WriteString("package ")
	b.WriteString(n.Name)
	b.WriteString(" is\n")
	for _, d := range n.Decls {
		b.WriteString("  ")
		printDecl(b, d, "  ")
		b.WriteByte('\n')
	}
	b.WriteString("end package;\n")
}

func printEntityDecl(b *strings.Builder, n *EntityDecl) {
	b.WriteString("entity ")
	b.WriteString(n.Name)
	b.WriteString(" is\n")
	if len(n.Generics) > 0 {
		b.WriteString("  generic (\n")
		printInterfaceList(b, n.Generics, "    ")
		b.WriteString("  );\n")
	}
	if len(n.Ports) > 0 {
		b.WriteString("  port (\n")
		printInterfaceList(b, n.Ports, "    ")
		b.WriteString("  );\n")
	}
	b.WriteString("end entity;\n")
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
			b.WriteString("  ")
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
	}
}

func printComponentDecl(b *strings.Builder, n *ComponentDecl, indent string) {
	b.WriteString("component ")
	b.WriteString(n.Name)
	b.WriteString(" is\n")
	if len(n.Generics) > 0 {
		b.WriteString(indent)
		b.WriteString("  generic (\n")
		printInterfaceList(b, n.Generics, indent+"    ")
		b.WriteString(indent)
		b.WriteString("  );\n")
	}
	if len(n.Ports) > 0 {
		b.WriteString(indent)
		b.WriteString("  port (\n")
		printInterfaceList(b, n.Ports, indent+"    ")
		b.WriteString(indent)
		b.WriteString("  );\n")
	}
	b.WriteString(indent)
	b.WriteString("end component;")
}

// printSubtypeIndication prints "mark" or "mark(constraint)" canonically.
// For a Paren constraint, the inner Lit text is emitted directly so the result
// is mark(inner) rather than mark((inner)).
func printSubtypeIndication(b *strings.Builder, mark string, constraint Expr) {
	b.WriteString(mark)
	if constraint == nil {
		return
	}
	b.WriteByte('(')
	if p, ok := constraint.(*Paren); ok {
		// Unwrap: emit the inner literal directly.
		printExpr(b, p.X)
	} else {
		printExpr(b, constraint)
	}
	b.WriteByte(')')
}

func printExpr(b *strings.Builder, e Expr) {
	switch n := e.(type) {
	case *Lit:
		b.WriteString(n.Text)
	case *Name:
		b.WriteString(n.Text)
	case *Range:
		printExpr(b, n.Left)
		b.WriteByte(' ')
		b.WriteString(n.Dir)
		b.WriteByte(' ')
		printExpr(b, n.Right)
	case *BinaryExpr:
		printExpr(b, n.X)
		b.WriteByte(' ')
		b.WriteString(n.Op)
		b.WriteByte(' ')
		printExpr(b, n.Y)
	case *CallOrIndex:
		printExpr(b, n.Prefix)
		b.WriteByte('(')
		for i, arg := range n.Args {
			if i > 0 {
				b.WriteString(", ")
			}
			printExpr(b, arg)
		}
		b.WriteByte(')')
	case *Paren:
		b.WriteByte('(')
		printExpr(b, n.X)
		b.WriteByte(')')
	}
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
	if x.Type() == reflect.TypeOf(NoPos) {
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
