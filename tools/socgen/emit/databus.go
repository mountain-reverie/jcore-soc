package emit

import (
	"fmt"
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
		rng := fmt.Sprintf("%08X-%08X", t.leaf.BaseAddr, t.leaf.BaseAddr+(2<<uint(t.leaf.LeftAddrBit))-1)
		return []vhdl.Stmt{
			&vhdl.Comment{Text: rng},
			&vhdl.ReturnStmt{Value: &vhdl.Ident{Name: lits[t.leaf.Name]}},
		}
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
// to enum literal identifiers.
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
		fn.Stmts = append(fn.Stmts,
			&vhdl.Comment{Text: `Assumes addr(31 downto 28) = x"a".`},
			&vhdl.Comment{Text: "Address decoding closer to CPU checks those bits."})
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
// the data_bus_i_t/data_bus_o_t array types, the devs_bus_i/o signals, then the
// decode_address function. The intermediate mux-output bus signals are declared
// separately by muxBusDecls (emitted before device_t).
func databusDecls(res *elaborate.Resolution) []vhdl.Decl {
	if res.DataBus == nil {
		return nil
	}
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

	decls = append(decls, decodeFunction(devs, lits, res.DataBus.DecodeMode == "simple"))
	return decls
}

// muxBusDecls returns the intermediate mux-output bus signal declarations
// (<Out>_periph_dbus_i/o). Current Clojure declares these before device_t
// (generate.clj :decls pbus-mux), so Devices() emits them ahead of databusDecls.
// Empty for single-master designs.
func muxBusDecls(res *elaborate.Resolution) []vhdl.Decl {
	if res.DataBus == nil {
		return nil
	}
	out := make([]vhdl.Decl, 0, 2*len(res.DataBus.MuxStages))
	for _, st := range res.DataBus.MuxStages {
		out = append(out,
			&vhdl.SignalDecl{Names: []string{st.Out + "_periph_dbus_i"}, SubtypeMark: "cpu_data_i_t"},
			&vhdl.SignalDecl{Names: []string{st.Out + "_periph_dbus_o"}, SubtypeMark: "cpu_data_o_t"},
		)
	}
	return out
}

// muxChainStmts emits the multi-master arbitration mux instantiations
// (generate.clj:403-453). Each MuxStage wires two input peripheral buses to an
// output bus via a multi_master_bus_mux/muxff entity. Single-master designs have
// no stages and emit nothing. The intermediate <Out>_periph_dbus_i/o signals are
// declared by muxBusDecls.
func muxChainStmts(res *elaborate.Resolution) []vhdl.Stmt {
	if res.DataBus == nil {
		return nil
	}
	out := make([]vhdl.Stmt, 0, len(res.DataBus.MuxStages))
	for _, s := range res.DataBus.MuxStages {
		inst := &vhdl.InstantiationStmt{
			Label:         s.Label,
			UnitKind:      vhdl.ENTITY,
			Unit:          "work." + s.Entity,
			KeepPortOrder: true,
			PortMap: []*vhdl.AssocElement{
				{Formal: "clk", Actual: &vhdl.Ident{Name: "clk_sys"}},
				{Formal: "rst", Actual: &vhdl.Ident{Name: "reset"}},
				{Formal: "m1_i", Actual: &vhdl.Ident{Name: s.In1 + "_periph_dbus_i"}},
				{Formal: "m1_o", Actual: &vhdl.Ident{Name: s.In1 + "_periph_dbus_o"}},
				{Formal: "m2_i", Actual: &vhdl.Ident{Name: s.In2 + "_periph_dbus_i"}},
				{Formal: "m2_o", Actual: &vhdl.Ident{Name: s.In2 + "_periph_dbus_o"}},
				{Formal: "slave_i", Actual: &vhdl.Ident{Name: s.Out + "_periph_dbus_i"}},
				{Formal: "slave_o", Actual: &vhdl.Ident{Name: s.Out + "_periph_dbus_o"}},
			},
		}
		out = append(out, inst)
	}
	return out
}

// databusStmts builds the data-bus concurrent statements in golden order
// (generate.clj:654-685; devices.vhd:87-95): the disconnected-bus loopbacks lead
// the concurrent region, then the `multiplex data bus to and from devices` block
// (the mux chain, the active_dev decode, the master read-back, the per-device
// bus_split for-generate, and the NONE loopback).
func databusStmts(res *elaborate.Resolution) []vhdl.Stmt {
	if res.DataBus == nil {
		return nil
	}
	master := res.DataBus.MasterBus
	out := make([]vhdl.Stmt, 0, 7+len(res.DataBus.Disconnected)+len(res.DataBus.MuxStages))

	// Disconnected peripheral buses lead the concurrent region (golden):
	//   <bus>_periph_dbus_i <= loopback_bus(<bus>_periph_dbus_o);
	if len(res.DataBus.Disconnected) > 0 {
		out = append(out, &vhdl.Comment{Text: "Disconnected peripheral buses"})
		for _, bus := range res.DataBus.Disconnected {
			out = append(out, concAssign(
				&vhdl.Ident{Name: bus + "_periph_dbus_i"},
				&vhdl.CallExpr{Fun: &vhdl.Ident{Name: "loopback_bus"}, Args: []*vhdl.AssocElement{{Actual: &vhdl.Ident{Name: bus + "_periph_dbus_o"}}}}))
		}
	}

	// The mux chain leads; the `multiplex data bus to and from devices` comment then
	// leads the active_dev/decode block (Clojure attaches it to active_dev, after the
	// mux). Single-master designs have no mux, so the comment position is unchanged.
	out = append(out, muxChainStmts(res)...)
	out = append(out, &vhdl.Comment{Text: "multiplex data bus to and from devices"})

	// active_dev <= decode_address(<master>_periph_dbus_o.a);
	out = append(out, concAssign(
		&vhdl.Ident{Name: "active_dev"},
		&vhdl.CallExpr{
			Fun:  &vhdl.Ident{Name: "decode_address"},
			Args: []*vhdl.AssocElement{{Actual: &vhdl.Ident{Name: master + "_periph_dbus_o.a"}}},
		}))

	// <master>_periph_dbus_i <= devs_bus_i(active_dev);
	out = append(out, concAssign(
		&vhdl.Ident{Name: master + "_periph_dbus_i"},
		&vhdl.CallExpr{
			Fun:  &vhdl.Ident{Name: "devs_bus_i"},
			Args: []*vhdl.AssocElement{{Actual: &vhdl.Ident{Name: "active_dev"}}},
		}))

	// bus_split : for dev in device_t'left to device_t'right generate
	//   devs_bus_o(dev) <= mask_data_o(<master>_periph_dbus_o, to_bit(dev = active_dev));
	out = append(out, &vhdl.GenerateStmt{
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
	out = append(out, concAssign(
		&vhdl.CallExpr{Fun: &vhdl.Ident{Name: "devs_bus_i"}, Args: []*vhdl.AssocElement{{Actual: &vhdl.Ident{Name: "NONE"}}}},
		&vhdl.CallExpr{Fun: &vhdl.Ident{Name: "loopback_bus"}, Args: []*vhdl.AssocElement{
			{Actual: &vhdl.CallExpr{Fun: &vhdl.Ident{Name: "devs_bus_o"}, Args: []*vhdl.AssocElement{{Actual: &vhdl.Ident{Name: "NONE"}}}}},
		}}))

	return out
}
