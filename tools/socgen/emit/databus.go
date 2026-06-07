package emit

import (
	"sort"
	"strconv"

	"github.com/j-core/jcore-soc/tools/socgen/vhdl"
)

// busDevice is the decode-relevant projection of a data-bus device.
type busDevice struct {
	Name        string
	BaseAddr    uint64
	LeftAddrBit int
}

// bin32 renders v as a 32-bit MSB-first binary string.
func bin32(v uint64) string {
	var b [32]byte
	for i := 0; i < 32; i++ {
		if v&(1<<uint(31-i)) != 0 {
			b[i] = '1'
		} else {
			b[i] = '0'
		}
	}
	return string(b[:])
}

// devicePrefix returns the left-most address bits selecting the device:
// bin(BaseAddr,32) minus the low (LeftAddrBit+1) device-internal bits
// (generate.clj:172-183). The `simple` argument is accepted for symmetry with
// the Clojure caller; simple-mode suffix trimming happens in decodeFunction (it
// needs the whole device set), so devicePrefix always returns the exact prefix.
func devicePrefix(d busDevice, _ bool) string {
	return bin32(d.BaseAddr)[:32-d.LeftAddrBit-1]
}

// trieNode is a binary radix trie node keyed by the '0'/'1' bytes of a device
// prefix. A node is a leaf when leaf != nil (mirrors the Clojure :name marker);
// otherwise kids holds the '0' and/or '1' children.
type trieNode struct {
	kids map[byte]*trieNode
	leaf *busDevice
}

func newTrie() *trieNode { return &trieNode{kids: map[byte]*trieNode{}} }

// insert places dev at the path given by its prefix bits (assoc-in).
func (t *trieNode) insert(prefix string, dev *busDevice) {
	node := t
	for i := 0; i < len(prefix); i++ {
		k := prefix[i]
		child, ok := node.kids[k]
		if !ok {
			child = newTrie()
			node.kids[k] = child
		}
		node = child
	}
	node.leaf = dev
}

// child returns the single-key subtrie with key k removed, i.e. the subtrie
// reached by following the OTHER key. Mirrors (dissoc trie \1) / (dissoc trie \0)
// followed by extract-prefix on the remaining single-child map.
func (t *trieNode) without(k byte) *trieNode {
	rem := newTrie()
	for kk, v := range t.kids {
		if kk != k {
			rem.kids[kk] = v
		}
	}
	return rem
}

// extractPrefix collapses outer single-child maps, returning the concatenated
// keys and the inner trie with those maps removed (generate.clj:222-232).
func extractPrefix(t *trieNode) (string, *trieNode) {
	prefix := ""
	for t.leaf == nil && len(t.kids) == 1 {
		var k byte
		var v *trieNode
		for kk, vv := range t.kids {
			k, v = kk, vv
		}
		prefix += string(k)
		t = v
	}
	return prefix, t
}

// addrIndex builds the name expression addr(i).
func addrIndex(i int) vhdl.Expr { return &vhdl.Ident{Name: "addr(" + strconv.Itoa(i) + ")"} }

// addrSlice builds the name expression addr(hi downto lo).
func addrSlice(hi, lo int) vhdl.Expr {
	return &vhdl.Ident{Name: "addr(" + strconv.Itoa(hi) + " downto " + strconv.Itoa(lo) + ")"}
}

// buildCond returns the boolean condition `addr(index) = '0'` (single bit) or
// `addr(index downto index-len+1) = "expect"` (multi-bit) (generate.clj:233-243).
func buildCond(index int, expect string) vhdl.Expr {
	if len(expect) == 1 {
		return &vhdl.BinaryExpr{
			X:  addrIndex(index),
			Op: vhdl.EQ,
			Y:  &vhdl.BasicLit{Kind: vhdl.CHARLIT, Value: "'" + expect + "'"},
		}
	}
	return &vhdl.BinaryExpr{
		X:  addrSlice(index, index-len(expect)+1),
		Op: vhdl.EQ,
		Y:  &vhdl.BasicLit{Kind: vhdl.STRINGLIT, Value: `"` + expect + `"`},
	}
}

// buildIfs renders a trie subtree into a slice of statements (generate.clj:244-275).
// A leaf yields `return <device-literal>;`. A branch splits the '0'-subtree (left)
// and '1'-subtree (right), extracts each prefix, and emits an if/elsif (or, when
// both checks reduce to a single distinguishing bit, an if/else with no elsif).
func buildIfs(offset int, t *trieNode, lits map[string]string) []vhdl.Stmt {
	index := 31 - offset
	if t.leaf != nil {
		return []vhdl.Stmt{&vhdl.ReturnStmt{Value: &vhdl.Ident{Name: lits[t.leaf.Name]}}}
	}

	leftPrefix, leftTrie := extractPrefix(t.without('1'))
	rightPrefix, rightTrie := extractPrefix(t.without('0'))

	leftStmts := buildIfs(offset+len(leftPrefix), leftTrie, lits)
	rightStmts := buildIfs(offset+len(rightPrefix), rightTrie, lits)

	if leftPrefix == "0" && rightPrefix == "1" {
		// avoid unnecessary elsif for simple 0-or-1 test
		return []vhdl.Stmt{&vhdl.IfStmt{
			Cond: buildCond(index, leftPrefix),
			Then: leftStmts,
			Else: rightStmts,
		}}
	}
	return []vhdl.Stmt{&vhdl.IfStmt{
		Cond: buildCond(index, leftPrefix),
		Then: leftStmts,
		Elsifs: []*vhdl.ElsifClause{{
			Cond:  buildCond(index, rightPrefix),
			Stmts: rightStmts,
		}},
	}}
}

// trimSuffix implements the simple-mode suffix trimming (generate.clj:189-217):
// it walks the prefix trie and, for each leaf, returns the shortest prefix that
// still distinguishes it from the other devices (collapsing trailing
// non-branching bits). Returns a map from device name to trimmed prefix.
func trimSuffix(t *trieNode, fullPrefix, prefix string, out map[string]string) {
	switch {
	case t.leaf != nil:
		out[t.leaf.Name] = fullPrefix
	case len(t.kids) == 1:
		var k byte
		var v *trieNode
		for kk, vv := range t.kids {
			k, v = kk, vv
		}
		trimSuffix(v, fullPrefix, prefix+string(k), out)
	default:
		for _, k := range sortedKids(t) {
			trimSuffix(t.kids[k], fullPrefix+prefix+string(k), "", out)
		}
	}
}

// sortedKids returns a node's child keys in deterministic ('0' then '1') order.
func sortedKids(t *trieNode) []byte {
	ks := make([]byte, 0, len(t.kids))
	for k := range t.kids {
		ks = append(ks, k)
	}
	sort.Slice(ks, func(i, j int) bool { return ks[i] < ks[j] })
	return ks
}

// devicePrefixes returns the per-device decode prefix map (device-addr-prefixes,
// generate.clj:160-218). In exact mode (simple==false) each prefix is the full
// devicePrefix; in simple mode trailing non-distinguishing bits are trimmed.
func devicePrefixes(devs []busDevice, simple bool) map[string]string {
	exact := make(map[string]string, len(devs))
	for i := range devs {
		exact[devs[i].Name] = devicePrefix(devs[i], simple)
	}
	if !simple {
		return exact
	}
	trie := newTrie()
	for i := range devs {
		trie.insert(exact[devs[i].Name], &devs[i])
	}
	out := make(map[string]string, len(devs))
	trimSuffix(trie, "", "", out)
	return out
}

// decodeFunction builds the `decode_address(addr) return device_t` function
// (create-decode-fn, generate.clj:220-329). lits maps device names (and "none")
// to enum literal identifiers. The leaf address-range comments emitted by the
// Clojure version are dropped (the printer cannot emit comments).
func decodeFunction(devs []busDevice, lits map[string]string, simple bool) *vhdl.SubprogramBody {
	param := &vhdl.InterfaceDecl{
		Names:       []string{"addr"},
		SubtypeMark: "std_logic_vector",
		Constraint: &vhdl.ParenExpr{
			X: &vhdl.Range{Left: intLit(31), Dir: vhdl.DOWNTO, Right: intLit(0)},
		},
	}
	fn := &vhdl.SubprogramBody{
		Designator: "decode_address",
		ReturnMark: "device_t",
		Params:     []*vhdl.InterfaceDecl{param},
	}

	if len(devs) > 0 {
		prefixes := devicePrefixes(devs, simple)
		trie := newTrie()
		for i := range devs {
			trie.insert(prefixes[devs[i].Name], &devs[i])
		}
		prefix, inner := extractPrefix(trie)

		if len(prefix) <= 4 {
			fn.Stmts = append(fn.Stmts, buildIfs(len(prefix), inner, lits)...)
		} else {
			// Wrap in a top-level test of the prefix bits past the leading 4
			// (which are checked closer to the CPU): if addr(27 downto ...) =
			// "<prefix[4:]>" then <build-ifs> end if. (generate.clj:321-327)
			fn.Stmts = append(fn.Stmts, &vhdl.IfStmt{
				Cond: buildCond(27, prefix[4:]),
				Then: buildIfs(len(prefix), inner, lits),
			})
		}
	}

	fn.Stmts = append(fn.Stmts, &vhdl.ReturnStmt{Value: &vhdl.Ident{Name: lits["none"]}})
	return fn
}
