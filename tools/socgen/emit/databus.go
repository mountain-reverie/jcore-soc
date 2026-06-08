package emit

import (
	"sort"
	"strconv"
	"strings"

	"github.com/j-core/jcore-soc/tools/socgen/elaborate"
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

// devLit returns the device_t enum literal for a data-bus device name
// (generate.clj:561: "DEV_" + upper-case(name)).
func devLit(name string) string { return "DEV_" + strings.ToUpper(name) }

// busDevicesOf collects the data-bus devices (dev.DataBus == true), sorted by
// name, projecting each onto the decode-relevant busDevice (generate.clj:551-558).
func busDevicesOf(res *elaborate.Resolution) []busDevice {
	devs := make([]busDevice, 0, len(res.Devices))
	for _, dev := range res.Devices {
		if !dev.DataBus || dev.BaseAddr == nil {
			continue
		}
		var left int
		if rc := res.Classes[lc(dev.Class)]; rc != nil {
			left = rc.LeftAddrBit
		}
		devs = append(devs, busDevice{Name: dev.Name, BaseAddr: *dev.BaseAddr, LeftAddrBit: left})
	}
	sort.Slice(devs, func(i, j int) bool { return devs[i].Name < devs[j].Name })
	return devs
}

// busLits maps each data-bus device name to its DEV literal, plus "none" -> "NONE"
// (generate.clj:562, the device_t literal map).
func busLits(res *elaborate.Resolution) map[string]string {
	lits := map[string]string{"none": "NONE"}
	for _, d := range busDevicesOf(res) {
		lits[d.Name] = devLit(d.Name)
	}
	return lits
}

// concAssign builds a simple concurrent signal assignment `target <= value;`.
func concAssign(target, value vhdl.Expr) *vhdl.ConcurrentSignalAssign {
	return &vhdl.ConcurrentSignalAssign{Target: target, Waveform: []*vhdl.WaveformElem{{Value: value}}}
}

// databusDecls builds the data-bus declarative items in golden order
// (generate.clj:559-585; devices.vhd:49-84): the device_t enum, active_dev signal,
// the data_bus_i_t/data_bus_o_t array types, the devs_bus_i/o signals, any
// intermediate mux-output bus signals, then the decode_address function.
func databusDecls(res *elaborate.Resolution) []vhdl.Decl {
	devs := busDevicesOf(res)
	lits := busLits(res)

	enumLits := []string{"NONE"}
	for _, d := range devs {
		enumLits = append(enumLits, devLit(d.Name))
	}

	decls := []vhdl.Decl{
		&vhdl.TypeDecl{Name: "device_t", Def: &vhdl.EnumDef{Lits: enumLits}},
		&vhdl.SignalDecl{Names: []string{"active_dev"}, SubtypeMark: "device_t"},
		&vhdl.TypeDecl{Name: "data_bus_i_t", Def: &vhdl.ArrayDef{Text: "array (device_t'left to device_t'right) of cpu_data_i_t"}},
		&vhdl.TypeDecl{Name: "data_bus_o_t", Def: &vhdl.ArrayDef{Text: "array (device_t'left to device_t'right) of cpu_data_o_t"}},
		&vhdl.SignalDecl{Names: []string{"devs_bus_i"}, SubtypeMark: "data_bus_i_t"},
		&vhdl.SignalDecl{Names: []string{"devs_bus_o"}, SubtypeMark: "data_bus_o_t"},
	}

	// Intermediate mux-output bus signals (none for single-master).
	for _, st := range res.DataBus.MuxStages {
		decls = append(decls,
			&vhdl.SignalDecl{Names: []string{st.Out + "_periph_dbus_i"}, SubtypeMark: "cpu_data_i_t"},
			&vhdl.SignalDecl{Names: []string{st.Out + "_periph_dbus_o"}, SubtypeMark: "cpu_data_o_t"},
		)
	}

	decls = append(decls, decodeFunction(devs, lits, res.DataBus.DecodeMode == "simple"))
	return decls
}

// muxChainStmts instantiates the multi-master arbitration mux chain (Task 7).
// For single-master designs there is no chain; returns nil.
func muxChainStmts(_ *elaborate.Resolution) []vhdl.Stmt { return nil }

// databusStmts builds the data-bus concurrent statements in golden order
// (generate.clj:654-685; devices.vhd:88-95): the (Task 7) mux chain, the
// active_dev decode, the master read-back, the per-device bus_split for-generate,
// the NONE loopback, and the disconnected-bus loopbacks.
func databusStmts(res *elaborate.Resolution) []vhdl.Stmt {
	master := res.DataBus.MasterBus
	stmts := make([]vhdl.Stmt, 0, 4+len(res.DataBus.Disconnected))

	// active_dev <= decode_address(<master>_periph_dbus_o.a);
	stmts = append(stmts, concAssign(
		&vhdl.Ident{Name: "active_dev"},
		&vhdl.CallExpr{
			Fun:  &vhdl.Ident{Name: "decode_address"},
			Args: []*vhdl.AssocElement{{Actual: &vhdl.Ident{Name: master + "_periph_dbus_o.a"}}},
		}))

	// <master>_periph_dbus_i <= devs_bus_i(active_dev);
	stmts = append(stmts, concAssign(
		&vhdl.Ident{Name: master + "_periph_dbus_i"},
		&vhdl.CallExpr{
			Fun:  &vhdl.Ident{Name: "devs_bus_i"},
			Args: []*vhdl.AssocElement{{Actual: &vhdl.Ident{Name: "active_dev"}}},
		}))

	// bus_split : for dev in device_t'left to device_t'right generate
	//   devs_bus_o(dev) <= mask_data_o(<master>_periph_dbus_o, to_bit(dev = active_dev));
	stmts = append(stmts, &vhdl.GenerateStmt{
		Label: "bus_split", Kind: vhdl.FOR, Param: "dev",
		Range: &vhdl.Ident{Name: "device_t'left to device_t'right"},
		Stmts: []vhdl.Stmt{concAssign(
			&vhdl.CallExpr{Fun: &vhdl.Ident{Name: "devs_bus_o"}, Args: []*vhdl.AssocElement{{Actual: &vhdl.Ident{Name: "dev"}}}},
			&vhdl.CallExpr{Fun: &vhdl.Ident{Name: "mask_data_o"}, Args: []*vhdl.AssocElement{
				{Actual: &vhdl.Ident{Name: master + "_periph_dbus_o"}},
				{Actual: &vhdl.CallExpr{Fun: &vhdl.Ident{Name: "to_bit"}, Args: []*vhdl.AssocElement{{Actual: &vhdl.BinaryExpr{X: &vhdl.Ident{Name: "dev"}, Op: vhdl.EQ, Y: &vhdl.Ident{Name: "active_dev"}}}}}},
			}}),
		},
	})

	// devs_bus_i(NONE) <= loopback_bus(devs_bus_o(NONE));
	stmts = append(stmts, concAssign(
		&vhdl.CallExpr{Fun: &vhdl.Ident{Name: "devs_bus_i"}, Args: []*vhdl.AssocElement{{Actual: &vhdl.Ident{Name: "NONE"}}}},
		&vhdl.CallExpr{Fun: &vhdl.Ident{Name: "loopback_bus"}, Args: []*vhdl.AssocElement{
			{Actual: &vhdl.CallExpr{Fun: &vhdl.Ident{Name: "devs_bus_o"}, Args: []*vhdl.AssocElement{{Actual: &vhdl.Ident{Name: "NONE"}}}}},
		}}))

	// Disconnected peripheral buses: <bus>_periph_dbus_i <= loopback_bus(<bus>_periph_dbus_o);
	disc := append([]string(nil), res.DataBus.Disconnected...)
	sort.Strings(disc)
	for _, bus := range disc {
		stmts = append(stmts, concAssign(
			&vhdl.Ident{Name: bus + "_periph_dbus_i"},
			&vhdl.CallExpr{Fun: &vhdl.Ident{Name: "loopback_bus"}, Args: []*vhdl.AssocElement{{Actual: &vhdl.Ident{Name: bus + "_periph_dbus_o"}}}}))
	}

	return append(muxChainStmts(res), stmts...)
}
