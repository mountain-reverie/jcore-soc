package iface

import (
	"strings"

	"github.com/j-core/jcore-soc/tools/socgen/vhdl"
)

// Library is the extracted interface model for a set of design files.
type Library struct {
	Entities       map[string]*Entity
	Packages       map[string]*Package
	Architectures  map[string][]*Architecture // keyed by ENTITY name
	Configurations map[string]*Configuration
	index          map[string]Symbol
}

// TypeRef is a type mark plus an optional constraint, kept AS WRITTEN.
type TypeRef struct {
	Mark       string
	Constraint vhdl.Expr // nil if unconstrained
}

func (t TypeRef) String() string { return vhdl.SubtypeString(t.Mark, t.Constraint) }

type Entity struct {
	Name            string
	Generics        []*Generic
	Ports           []*Port
	PeripheralBuses []*PeripheralBus
}

// PeripheralBus is an entity `group <name> : peripheral_bus(<ports>)` declaration:
// a bus master and the (master-relative) data-bus port names it owns.
type PeripheralBus struct {
	Name  string
	Ports []string
}

type Port struct {
	Name       string
	Dir        string // "in"|"out"|"inout"|"buffer"|"linkage"|"" (default in)
	Type       TypeRef
	GlobalName string // soc_port_global_name attr OR global_ports group membership (bare id); "" if none
	LocalName  string // soc_port_local_name attr OR local_ports group membership; "" if none
	IRQ        bool   // soc_port_irq attr (the device's interrupt output port)
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

func (l *Library) Package(name string) (*Package, bool) {
	p, ok := l.Packages[lower(name)]
	return p, ok
}

func (l *Library) ResolveType(name string) (*TypeEntry, bool) {
	s, ok := l.index[lower(name)]
	if !ok || (s.Kind != "type" && s.Kind != "subtype") {
		return nil, false
	}
	pkg := l.Packages[lower(s.Package)]
	for _, te := range pkg.Types {
		if lower(te.Name) == lower(name) {
			return te, true
		}
	}
	return nil, false
}

// TypePackage returns the name of the package that declares the named type or
// subtype, and whether it was found. Types not in any parsed (work) package
// (e.g. std_logic, unsigned) return ("", false).
func (l *Library) TypePackage(name string) (string, bool) {
	s, ok := l.index[lower(name)]
	if !ok || (s.Kind != "type" && s.Kind != "subtype") {
		return "", false
	}
	return s.Package, true
}

func (l *Library) Configuration(name string) (*Configuration, bool) {
	c, ok := l.Configurations[lower(name)]
	return c, ok
}

// IsConstant reports whether name is a constant declared in a parsed package.
// A nil library (a failed board load) reports false.
func (l *Library) IsConstant(name string) bool {
	if l == nil {
		return false
	}
	s, ok := l.index[lower(name)]
	return ok && s.Kind == "constant"
}

func lower(s string) string { return strings.ToLower(s) }
