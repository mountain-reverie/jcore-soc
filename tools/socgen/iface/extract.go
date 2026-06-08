package iface

import (
	"errors"

	"github.com/j-core/jcore-soc/tools/socgen/vhdl"
)

// Extract builds the interface model from parsed design files. It never panics;
// problems (duplicate names) are returned as a joined error alongside a
// best-effort Library.
func Extract(files []*vhdl.DesignFile) (*Library, error) {
	lib := &Library{
		Entities:       map[string]*Entity{},
		Packages:       map[string]*Package{},
		Architectures:  map[string][]*Architecture{},
		Configurations: map[string]*Configuration{},
		index:          map[string]Symbol{},
	}
	var errs []error
	for _, df := range files {
		for _, u := range df.Units {
			switch n := u.(type) {
			case *vhdl.EntityDecl:
				errs = append(errs, lib.addEntity(n))
			case *vhdl.ArchitectureBody:
				lib.addArchitecture(n)
			case *vhdl.PackageDecl:
				errs = append(errs, lib.addPackage(n))
			case *vhdl.ConfigurationDecl:
				errs = append(errs, lib.addConfiguration(n))
			}
		}
	}
	return lib, errors.Join(errs...)
}

func (l *Library) addEntity(n *vhdl.EntityDecl) error {
	key := lower(n.Name)
	var err error
	if _, dup := l.Entities[key]; dup {
		err = &DuplicateError{Kind: ErrDuplicateDecl, Decl: "entity", Symbol: n.Name}
	}
	ports := toPorts(n.Ports)
	applySocPortNames(ports, n.Decls)
	l.Entities[key] = &Entity{
		Name:            n.Name,
		Generics:        toGenerics(n.Generics),
		Ports:           ports,
		PeripheralBuses: toPeripheralBuses(n.Decls),
	}
	return err
}

func (l *Library) addArchitecture(n *vhdl.ArchitectureBody) {
	key := lower(n.Entity)
	l.Architectures[key] = append(l.Architectures[key], &Architecture{
		Name: n.Name, Entity: n.Entity, Node: n,
	})
}

func (l *Library) addPackage(n *vhdl.PackageDecl) error {
	key := lower(n.Name)
	var errs []error
	if _, dup := l.Packages[key]; dup {
		errs = append(errs, &DuplicateError{Kind: ErrDuplicateDecl, Decl: "package", Symbol: n.Name})
	}
	p := &Package{Name: n.Name}
	for _, d := range n.Decls {
		switch dd := d.(type) {
		case *vhdl.ConstantDecl:
			for _, cn := range dd.Names {
				p.Constants = append(p.Constants, &Constant{
					Name:  cn,
					Type:  TypeRef{Mark: dd.SubtypeMark, Constraint: dd.Constraint},
					Value: dd.Default,
				})
				errs = append(errs, l.addSymbol(cn, n.Name, "constant", dd))
			}
		case *vhdl.TypeDecl:
			p.Types = append(p.Types, &TypeEntry{Name: dd.Name, Node: dd})
			errs = append(errs, l.addSymbol(dd.Name, n.Name, "type", dd))
		case *vhdl.SubtypeDecl:
			p.Types = append(p.Types, &TypeEntry{Name: dd.Name, Node: dd})
			errs = append(errs, l.addSymbol(dd.Name, n.Name, "subtype", dd))
		case *vhdl.ComponentDecl:
			p.Components = append(p.Components, &Component{
				Name:     dd.Name,
				Generics: toGenerics(dd.Generics),
				Ports:    toPorts(dd.Ports),
			})
			errs = append(errs, l.addSymbol(dd.Name, n.Name, "component", dd))
		}
	}
	l.Packages[key] = p
	return errors.Join(errs...)
}

func (l *Library) addSymbol(name, pkg, kind string, node vhdl.Node) error {
	key := lower(name)
	var err error
	if prev, dup := l.index[key]; dup {
		err = &DuplicateError{Kind: ErrDuplicateSymbol, Symbol: name, Pkg: pkg, AlsoIn: prev.Package}
	}
	l.index[key] = Symbol{Package: pkg, Kind: kind, Node: node}
	return err
}

func toPorts(ids []*vhdl.InterfaceDecl) []*Port {
	var out []*Port
	for _, d := range ids {
		for _, name := range d.Names {
			out = append(out, &Port{
				Name: name,
				Dir:  d.Mode,
				Type: TypeRef{Mark: d.SubtypeMark, Constraint: d.Constraint},
			})
		}
	}
	return out
}

func (l *Library) addConfiguration(n *vhdl.ConfigurationDecl) error {
	key := lower(n.Name)
	var err error
	if _, dup := l.Configurations[key]; dup {
		err = &DuplicateError{Kind: ErrDuplicateDecl, Decl: "configuration", Symbol: n.Name}
	}
	arch := ""
	if n.Block != nil {
		arch = n.Block.Spec
	}
	l.Configurations[key] = &Configuration{Name: n.Name, Entity: n.Entity, Arch: arch, Node: n}
	return err
}

func toPeripheralBuses(decls []vhdl.Decl) []*PeripheralBus {
	var out []*PeripheralBus
	for _, d := range decls {
		if g, ok := d.(*vhdl.GroupDecl); ok && lower(g.TemplateMark) == "peripheral_bus" {
			out = append(out, &PeripheralBus{Name: g.Name, Ports: append([]string(nil), g.Constituents...)})
		}
	}
	return out
}

// applySocPortNames fills Port.GlobalName/LocalName from global_ports/local_ports
// group memberships (bare own id) and soc_port_global_name/soc_port_local_name
// attribute specs (lowercased string value). Attributes override groups.
func applySocPortNames(ports []*Port, decls []vhdl.Decl) {
	byName := map[string]*Port{}
	for _, p := range ports {
		byName[lower(p.Name)] = p
	}
	// groups first: a member's name = its own bare lowercased id
	for _, d := range decls {
		g, ok := d.(*vhdl.GroupDecl)
		if !ok {
			continue
		}
		for _, c := range g.Constituents {
			p := byName[lower(c)]
			if p == nil {
				continue
			}
			switch lower(g.TemplateMark) {
			case "global_ports":
				p.GlobalName = lower(c)
			case "local_ports":
				p.LocalName = lower(c)
			}
		}
	}
	// attributes override groups
	for _, d := range decls {
		a, ok := d.(*vhdl.AttributeSpec)
		if !ok || a.EntityClass != vhdl.SIGNAL {
			continue
		}
		val, ok := socPortString(a.Value)
		if !ok {
			continue
		}
		for _, e := range a.Entities {
			p := byName[lower(e)]
			if p == nil {
				continue
			}
			switch lower(a.Name) {
			case "soc_port_global_name":
				p.GlobalName = val
			case "soc_port_local_name":
				p.LocalName = val
			}
		}
	}
}

// socPortString extracts a lowercased string-literal value (quotes stripped) from
// an attribute-spec value expression.
func socPortString(e vhdl.Expr) (string, bool) {
	lit, ok := e.(*vhdl.BasicLit)
	if !ok || lit.Kind != vhdl.STRINGLIT {
		return "", false
	}
	s := lit.Value
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		s = s[1 : len(s)-1]
	}
	return lower(s), true
}

func toGenerics(ids []*vhdl.InterfaceDecl) []*Generic {
	var out []*Generic
	for _, d := range ids {
		for _, name := range d.Names {
			out = append(out, &Generic{
				Name:    name,
				Type:    TypeRef{Mark: d.SubtypeMark, Constraint: d.Constraint},
				Default: d.Default,
			})
		}
	}
	return out
}
