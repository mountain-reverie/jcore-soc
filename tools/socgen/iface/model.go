package iface

import (
	"fmt"
	"strings"

	"github.com/j-core/jcore-soc/tools/socgen/vhdl"
)

// Library is the extracted interface model for a set of design files.
type Library struct {
	Entities       map[string]*Entity
	Packages       map[string]*Package        // populated in Task 2
	Architectures  map[string][]*Architecture // keyed by ENTITY name
	Configurations map[string]*Configuration  // populated in Task 3
	index          map[string]Symbol          // populated in Task 2
}

// TypeRef is a type mark plus an optional constraint, kept AS WRITTEN.
type TypeRef struct {
	Mark       string
	Constraint vhdl.Expr // nil if unconstrained
}

func (t TypeRef) String() string { return vhdl.SubtypeString(t.Mark, t.Constraint) }

type Entity struct {
	Name     string
	Generics []*Generic
	Ports    []*Port
}

type Port struct {
	Name string
	Dir  string // "in"|"out"|"inout"|"buffer"|"linkage"|"" (default in)
	Type TypeRef
}

type Generic struct {
	Name    string
	Type    TypeRef
	Default vhdl.Expr // nil if no default
}

type Architecture struct {
	Name   string
	Entity string
	Node   *vhdl.ArchitectureBody
}

type Package struct {
	Name       string
	Constants  []*Constant
	Types      []*TypeEntry
	Components []*Component
}

type Constant struct {
	Name  string
	Type  TypeRef
	Value vhdl.Expr
}

type TypeEntry struct {
	Name string
	Node vhdl.Decl
}

type Component struct {
	Name     string
	Generics []*Generic
	Ports    []*Port
}

type Symbol struct {
	Package string
	Kind    string
	Node    vhdl.Node
}

type Configuration struct {
	Name   string
	Entity string
	Arch   string
	Node   *vhdl.ConfigurationDecl
}

func (l *Library) Entity(name string) (*Entity, bool) {
	e, ok := l.Entities[lower(name)]
	return e, ok
}

func (l *Library) ArchitecturesOf(entity string) []*Architecture {
	return l.Architectures[lower(entity)]
}

func lower(s string) string { return strings.ToLower(s) }

func dupErr(kind, name string) error { return fmt.Errorf("duplicate %s declaration: %s", kind, name) }
