package iface

import (
	"fmt"

	"github.com/j-core/jcore-soc/tools/socgen/vhdl"
)

// Extract builds the interface model from parsed design files. It never panics;
// problems (duplicate names) are returned as errors alongside a best-effort Library.
func Extract(files []*vhdl.DesignFile) (*Library, []error) {
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
				errs = lib.addEntity(n, errs)
			case *vhdl.ArchitectureBody:
				lib.addArchitecture(n)
			case *vhdl.PackageDecl:
				errs = lib.addPackage(n, errs)
			case *vhdl.ConfigurationDecl:
				errs = lib.addConfiguration(n, errs)
			}
		}
	}
	return lib, errs
}

func (l *Library) addEntity(n *vhdl.EntityDecl, errs []error) []error {
	key := lower(n.Name)
	if _, dup := l.Entities[key]; dup {
		errs = append(errs, dupErr("entity", n.Name))
	}
	l.Entities[key] = &Entity{
		Name:     n.Name,
		Generics: toGenerics(n.Generics),
		Ports:    toPorts(n.Ports),
	}
	return errs
}

func (l *Library) addArchitecture(n *vhdl.ArchitectureBody) {
	key := lower(n.Entity)
	l.Architectures[key] = append(l.Architectures[key], &Architecture{
		Name: n.Name, Entity: n.Entity, Node: n,
	})
}

func (l *Library) addPackage(n *vhdl.PackageDecl, errs []error) []error {
	key := lower(n.Name)
	if _, dup := l.Packages[key]; dup {
		errs = append(errs, dupErr("package", n.Name))
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
				errs = l.addSymbol(cn, n.Name, "constant", dd, errs)
			}
		case *vhdl.TypeDecl:
			p.Types = append(p.Types, &TypeEntry{Name: dd.Name, Node: dd})
			errs = l.addSymbol(dd.Name, n.Name, "type", dd, errs)
		case *vhdl.SubtypeDecl:
			p.Types = append(p.Types, &TypeEntry{Name: dd.Name, Node: dd})
			errs = l.addSymbol(dd.Name, n.Name, "subtype", dd, errs)
		case *vhdl.ComponentDecl:
			p.Components = append(p.Components, &Component{
				Name:     dd.Name,
				Generics: toGenerics(dd.Generics),
				Ports:    toPorts(dd.Ports),
			})
			errs = l.addSymbol(dd.Name, n.Name, "component", dd, errs)
		}
	}
	l.Packages[key] = p
	return errs
}

func (l *Library) addSymbol(name, pkg, kind string, node vhdl.Node, errs []error) []error {
	key := lower(name)
	if prev, dup := l.index[key]; dup {
		errs = append(errs, fmt.Errorf("duplicate symbol %q in package %s (also in %s)", name, pkg, prev.Package))
	}
	l.index[key] = Symbol{Package: pkg, Kind: kind, Node: node}
	return errs
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

func (l *Library) addConfiguration(n *vhdl.ConfigurationDecl, errs []error) []error {
	key := lower(n.Name)
	if _, dup := l.Configurations[key]; dup {
		errs = append(errs, dupErr("configuration", n.Name))
	}
	arch := ""
	if n.Block != nil {
		arch = n.Block.Spec
	}
	l.Configurations[key] = &Configuration{Name: n.Name, Entity: n.Entity, Arch: arch, Node: n}
	return errs
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
