// Package dts provides a Device Tree Source (DTS) AST and a printer that
// reproduces the exact textual format of the golden board.dts files (tabs,
// blank lines, trailing comments). It is a port of the Clojure
// soc-gen.plugins.device-tree to-str / prop-val-str / props-to-str functions.
package dts

import (
	"strconv"
	"strings"
)

// Node is a device-tree node: a name, an ordered list of properties, and
// ordered child nodes.
type Node struct {
	Name     string
	Props    []*Prop
	Children []*Node
}

// Prop is a device-tree property.
//
// A Prop with an empty Name is a blank-line separator within a node's property
// list (matching the Clojure empty-vector formatting behavior).
//
// A Prop with no Values is a flag property, rendered as "name;".
type Prop struct {
	Name   string  // "" => a blank-line separator
	Values []Value // empty => flag prop ("name;")
	Cmt    string  // trailing "// cmt"
}

// Value is a device-tree property value.
type Value interface{ render() string }

// Str is a quoted-string value.
type Str string

func (s Str) render() string { return strconv.Quote(string(s)) }

// Cells is a <...> cell-array value. Refs (phandle references, rendered as
// "&name") are emitted before Nums. When Hex is set, Nums are rendered in hex.
type Cells struct {
	Nums []uint64
	Hex  bool
	Refs []string // &name phandle refs (rendered before Nums)
}

func (c Cells) render() string {
	parts := make([]string, 0, len(c.Refs)+len(c.Nums))
	for _, r := range c.Refs {
		parts = append(parts, "&"+r)
	}
	for _, n := range c.Nums {
		if c.Hex {
			parts = append(parts, "0x"+strconv.FormatUint(n, 16))
		} else {
			parts = append(parts, strconv.FormatUint(n, 10))
		}
	}
	return "<" + strings.Join(parts, " ") + ">"
}

// Ref is a bare phandle reference value ("&name", no surrounding "<...>"), used
// by nodes like "aliases" whose values are phandles rather than cell arrays
// (contrast Cells.Refs, which renders "<&name>").
type Ref string

func (r Ref) render() string { return "&" + string(r) }

// Bytes is a [..] byte-string value, rendered as space-separated two-digit hex.
type Bytes []byte

func (b Bytes) render() string {
	parts := make([]string, len(b))
	for i, x := range b {
		h := strconv.FormatUint(uint64(x), 16)
		if len(h) == 1 {
			h = "0" + h
		}
		parts[i] = h
	}
	return "[" + strings.Join(parts, " ") + "]"
}

// Print renders a complete device tree, prefixed with the "/dts-v1/;" header.
// root must be non-nil.
func Print(root *Node) string { return "/dts-v1/;\n\n" + renderNode(0, root) }

// renderNode renders a single node (and its subtree) at the given indent depth.
//
// Formatting (matching the golden / Clojure to-str):
//   - "name {\n"
//   - each named property on its own indented line; an empty-name Prop emits a
//     single blank line (a formatting separator)
//   - a blank line precedes each child node
//   - "};\n" closes the node at the node's own indent
func renderNode(indent int, n *Node) string {
	ind := strings.Repeat("\t", indent)
	var body strings.Builder
	for _, p := range n.Props {
		if p.Name == "" {
			body.WriteString("\n")
			continue
		}
		body.WriteString(ind + "\t" + renderProp(p) + "\n")
	}
	for _, c := range n.Children {
		if c == nil {
			continue
		}
		body.WriteString("\n")
		body.WriteString(renderNode(indent+1, c))
	}

	var b strings.Builder
	b.WriteString(ind + n.Name + " {")
	if body.Len() > 0 {
		// Non-empty body: "{\n" + body + "ind};\n" (matching Clojure to-str).
		b.WriteString("\n")
		b.WriteString(body.String())
		b.WriteString(ind + "};\n")
	} else {
		// Empty body: "{ };\n" (Clojure uses a single space before "};").
		b.WriteString(" };\n")
	}
	return b.String()
}

func renderProp(p *Prop) string {
	s := p.Name
	if len(p.Values) > 0 {
		vs := make([]string, len(p.Values))
		for i, v := range p.Values {
			vs[i] = v.render()
		}
		s += " = " + strings.Join(vs, ", ")
	}
	s += ";"
	if p.Cmt != "" {
		s += " // " + p.Cmt
	}
	return s
}
