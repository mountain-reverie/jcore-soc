package iface

import "github.com/j-core/jcore-soc/tools/socgen/vhdl"

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
