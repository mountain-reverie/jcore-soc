package design

import (
	"os"
	"path/filepath"
	"slices"

	"gopkg.in/yaml.v3"
)

const maxIncludeDepth = 16

// resolveTree resolves !include nodes (relative to dir) in place and applies
// merge semantics (<< merge keys, !remove). stack guards against include cycles.
// It is called from Load; after it returns the tree is ready for Decode (no
// !include, <<, or !remove nodes remain).
func resolveTree(n *yaml.Node, dir string, stack []string) error {
	if err := resolveIncludes(n, dir, stack); err != nil {
		return err
	}
	stripRemove(n)
	return nil
}

// resolveIncludes is the recursive workhorse: resolves !include scalars and
// applies << merge keys. !remove sentinels are left in place so that
// deepMergeInto can use them as tombstones; stripRemove cleans them up after.
func resolveIncludes(n *yaml.Node, dir string, stack []string) error {
	if n == nil {
		return nil
	}
	// 1. !include scalar -> splice the referenced file's processed root in place.
	if n.Kind == yaml.ScalarNode && n.Tag == "!include" {
		return spliceInclude(n, dir, stack)
	}
	// 2. recurse into children first (so nested includes resolve), then merge maps.
	for _, c := range n.Content {
		if err := resolveIncludes(c, dir, stack); err != nil {
			return err
		}
	}
	if n.Kind == yaml.MappingNode {
		return mergeMapping(n)
	}
	return nil
}

// stripRemove recursively removes any k/v pairs where v is tagged !remove.
// This final pass cleans up any tombstones that were not consumed by deepMergeInto
// (e.g. standalone !remove in a map that had no << merge or whose inherited key
// was absent in the base).
func stripRemove(n *yaml.Node) {
	if n == nil {
		return
	}
	if n.Kind == yaml.MappingNode {
		var out []*yaml.Node
		for i := 0; i+1 < len(n.Content); i += 2 {
			k, v := n.Content[i], n.Content[i+1]
			if v.Kind == yaml.ScalarNode && v.Tag == "!remove" {
				continue
			}
			out = append(out, k, v)
		}
		n.Content = out
	}
	for _, c := range n.Content {
		stripRemove(c)
	}
}

func spliceInclude(n *yaml.Node, dir string, stack []string) error {
	if len(stack) >= maxIncludeDepth {
		return &IncludeError{Kind: ErrIncludeDepth, Path: n.Value}
	}
	p := filepath.Join(dir, n.Value)
	abs, aerr := filepath.Abs(p)
	if aerr != nil {
		return &IncludeError{Kind: ErrIncludeRead, Path: p, Err: aerr}
	}
	if slices.Contains(stack, abs) {
		return &IncludeError{Kind: ErrIncludeCycle, Path: abs}
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return &IncludeError{Kind: ErrIncludeRead, Path: p, Err: err}
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return &IncludeError{Kind: ErrIncludeRead, Path: p, Err: err}
	}
	root := &doc
	if doc.Kind == yaml.DocumentNode && len(doc.Content) == 1 {
		root = doc.Content[0]
	}
	if err := resolveTree(root, filepath.Dir(p), append(stack, abs)); err != nil {
		return err
	}
	*n = *root // replace the !include scalar with the included content
	return nil
}

// mergeMapping applies a "<<" merge key (deep-merge its value map UNDER the
// sibling keys) and drops keys whose value is !remove. yaml mapping Content is
// a flat [k1,v1,k2,v2,...] slice.
func mergeMapping(n *yaml.Node) error {
	var out []*yaml.Node // rebuilt [k,v,...]
	var mergeVal *yaml.Node
	for i := 0; i+1 < len(n.Content); i += 2 {
		k, v := n.Content[i], n.Content[i+1]
		if k.Value == "<<" {
			if mergeVal != nil {
				return &SpecError{Line: n.Line, Msg: "multiple << merge keys in one mapping"}
			}
			mergeVal = v
			continue
		}
		out = append(out, k, v)
	}
	n.Content = out
	if mergeVal != nil {
		mv := mergeVal
		if mv.Kind == yaml.AliasNode {
			mv = mv.Alias
		}
		if mv == nil || mv.Kind != yaml.MappingNode {
			return &SpecError{Line: n.Line, Msg: "<< value must be a mapping"}
		}
		deepMergeInto(n, mv) // siblings (already in n) win over merged
	}
	return nil
}

// deepMergeInto merges base's keys into dst WITHOUT overriding keys dst already
// has; for keys present in both as mappings, merge recursively; for sequences,
// concatenate (dst first); a !remove in dst deletes a key inherited from base.
func deepMergeInto(dst, base *yaml.Node) {
	for i := 0; i+1 < len(base.Content); i += 2 {
		bk, bv := base.Content[i], base.Content[i+1]
		di := mapIndex(dst, bk.Value)
		if di < 0 {
			dst.Content = append(dst.Content, bk, bv)
			continue
		}
		dv := dst.Content[di+1]
		if dv.Kind == yaml.ScalarNode && dv.Tag == "!remove" {
			dst.Content = append(dst.Content[:di], dst.Content[di+2:]...) // remove inherited key
			continue
		}
		if dv.Kind == yaml.MappingNode && bv.Kind == yaml.MappingNode {
			deepMergeInto(dv, bv)
		} else if dv.Kind == yaml.SequenceNode && bv.Kind == yaml.SequenceNode {
			dv.Content = append(dv.Content, bv.Content...) // concat
		}
		// otherwise dst's scalar wins (override) — leave as is.
	}
}

func mapIndex(m *yaml.Node, key string) int {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return i
		}
	}
	return -1
}
