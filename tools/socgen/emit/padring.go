package emit

import (
	"sort"

	"github.com/j-core/jcore-soc/tools/socgen/elaborate"
	"github.com/j-core/jcore-soc/tools/socgen/vhdl"
)

// bufferGenericAttrs are the attributes that belong to the I/O buffer instances
// (P5d-b), not the pad port; they are excluded from pad-port attributes.
var bufferGenericAttrs = map[string]bool{"iostandard": true, "drive": true, "slew": true, "diff_term": true}

// sortedPins returns res.Pins sorted by net for deterministic emission.
func sortedPins(res *elaborate.Resolution) []*elaborate.ResolvedPin {
	ps := append([]*elaborate.ResolvedPin(nil), res.Pins...)
	sort.Slice(ps, func(i, j int) bool { return ps[i].Net < ps[j].Net })
	return ps
}

// padRingPorts builds the pad_ring entity ports: one `pin_<net> : <dir> std_logic`
// per resolved pin (both legs of a differential pair are separate pins).
func padRingPorts(res *elaborate.Resolution) []*vhdl.InterfaceDecl {
	pins := sortedPins(res)
	out := make([]*vhdl.InterfaceDecl, 0, len(pins))
	for _, p := range pins {
		dir := p.PadDir
		if dir == "" {
			dir = "in"
		}
		out = append(out, &vhdl.InterfaceDecl{Names: []string{"pin_" + p.Net}, Mode: dir, SubtypeMark: "std_logic"})
	}
	return out
}

// pinAttrs builds the pad-port attribute declarations + specifications: `loc` (the
// pad) plus any non-buffer-generic attribute (e.g. tig). Buffer generics
// (iostandard/drive/slew/diff_term) belong to the I/O buffers (P5d-b).
func pinAttrs(res *elaborate.Resolution) []vhdl.Decl {
	pins := sortedPins(res)
	// collect distinct attribute names (in deterministic order) for the decls.
	declOrder := []string{"loc"}
	seen := map[string]bool{"loc": true}
	type spec struct{ name, ent, val string }
	var specs []spec
	for _, p := range pins {
		port := "pin_" + p.Net
		if p.Pad != "" {
			specs = append(specs, spec{"loc", port, p.Pad})
		}
		// other attrs (sorted) excluding buffer generics and loc.
		var keys []string
		for k := range p.Attrs {
			lk := lc(k)
			if lk == "loc" || bufferGenericAttrs[lk] {
				continue
			}
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			lk := lc(k)
			if !seen[lk] {
				seen[lk] = true
				declOrder = append(declOrder, lk)
			}
			specs = append(specs, spec{lk, port, p.Attrs[k].Text})
		}
	}
	out := make([]vhdl.Decl, 0, len(declOrder)+len(specs))
	for _, name := range declOrder {
		out = append(out, &vhdl.AttributeDecl{Name: name, TypeMark: "string"})
	}
	for _, s := range specs {
		out = append(out, &vhdl.AttributeSpec{
			Name: s.name, Entities: []string{s.ent}, EntityClass: vhdl.SIGNAL,
			Value: &vhdl.BasicLit{Kind: vhdl.STRINGLIT, Value: `"` + s.val + `"`},
		})
	}
	return out
}
