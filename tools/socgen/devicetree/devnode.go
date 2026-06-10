package devicetree

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/j-core/jcore-soc/tools/socgen/design"
	"github.com/j-core/jcore-soc/tools/socgen/dts"
)

// cacheCompat is the compatible string whose interrupt info is described on the
// ipi node instead of the device node; faithful to device_tree.clj cache-compat.
const cacheCompat = "jcore,cache"

// toUint64 coerces a loosely-typed (YAML-decoded) numeric value to uint64. YAML
// integers decode into Go int via yaml.v3; the other integer kinds are accepted
// defensively. Scientific-notation integers (e.g. 25e6) decode as float64 and
// are accepted when non-negative and integral. Negative signed values are
// rejected (rather than silently wrapping). A numeric-looking string (a 0x-hex
// that decoded as a string) is also parsed so cells contexts stay robust.
func toUint64(v any) (uint64, bool) {
	switch n := v.(type) {
	case int:
		return signedToUint64(int64(n))
	case int8:
		return signedToUint64(int64(n))
	case int16:
		return signedToUint64(int64(n))
	case int32:
		return signedToUint64(int64(n))
	case int64:
		return signedToUint64(n)
	case float64:
		if n >= 0 && n == math.Trunc(n) && n <= math.MaxUint64 {
			return uint64(n), true
		}
		return 0, false
	case uint:
		return uint64(n), true
	case uint8:
		return uint64(n), true
	case uint16:
		return uint64(n), true
	case uint32:
		return uint64(n), true
	case uint64:
		return n, true
	case string:
		var u uint64
		if _, err := fmt.Sscanf(strings.TrimSpace(n), "0x%x", &u); err == nil {
			return u, true
		}
		if _, err := fmt.Sscanf(strings.TrimSpace(n), "%d", &u); err == nil {
			return u, true
		}
	}
	return 0, false
}

// signedToUint64 converts a signed integer to uint64, rejecting negatives so a
// negative value does not silently wrap to a huge uint64.
func signedToUint64(n int64) (uint64, bool) {
	if n < 0 {
		return 0, false
	}
	return uint64(n), true
}

// isAllNumbers reports whether every element of xs coerces to a uint64.
func isAllNumbers(xs []any) bool {
	for _, x := range xs {
		if _, ok := toUint64(x); !ok {
			return false
		}
	}
	return true
}

// dtValue converts a loosely-typed dt-prop value (NOT a flag — bool false is
// handled by the caller) to the corresponding dts value(s):
//   - string -> a single Str ("s")
//   - []any of all-numbers -> a single decimal Cells (<n n>)
//   - []any of all-strings, all phandle refs ("&x") -> a single Cells of Refs
//     (<&x>); otherwise a comma-joined list of Str ("a", "b")
//   - bare number -> a single decimal Cells (<n>)
func dtValue(v any) []dts.Value {
	switch x := v.(type) {
	case string:
		return []dts.Value{dts.Str(x)}
	case []any:
		if len(x) > 0 && isAllNumbers(x) {
			nums := make([]uint64, len(x))
			for i, e := range x {
				nums[i], _ = toUint64(e)
			}
			return []dts.Value{dts.Cells{Nums: nums}}
		}
		// All-string lists: phandle refs ("&name") collapse into one <&a &b>
		// cells value (faithful to the golden clocks = <&bus_clock>;); plain
		// strings become a comma-joined list of quoted strings.
		if refs, ok := allPhandleRefs(x); ok {
			return []dts.Value{dts.Cells{Refs: refs}}
		}
		vals := make([]dts.Value, 0, len(x))
		for _, e := range x {
			if s, ok := e.(string); ok {
				vals = append(vals, dts.Str(s))
			} else if n, ok := toUint64(e); ok {
				vals = append(vals, dts.Cells{Nums: []uint64{n}})
			}
		}
		return vals
	default:
		if n, ok := toUint64(v); ok {
			return []dts.Value{dts.Cells{Nums: []uint64{n}}}
		}
	}
	return nil
}

// allPhandleRefs reports whether xs is a non-empty list of "&name" phandle-ref
// strings, returning the names with the leading & stripped.
func allPhandleRefs(xs []any) ([]string, bool) {
	if len(xs) == 0 {
		return nil, false
	}
	refs := make([]string, 0, len(xs))
	for _, x := range xs {
		s, ok := x.(string)
		if !ok || !strings.HasPrefix(s, "&") {
			return nil, false
		}
		refs = append(refs, strings.TrimPrefix(s, "&"))
	}
	return refs, true
}

// dtProp converts one dt-prop key/value to a dts.Prop. Any bool value (false or
// true) renders as a flag property ("k;", no values): false short-circuits here,
// and true falls through to dtValue which yields nil values; everything else
// uses dtValue.
func dtProp(k string, v any) *dts.Prop {
	if b, ok := v.(bool); ok && !b {
		return &dts.Prop{Name: k}
	}
	return &dts.Prop{Name: k, Values: dtValue(v)}
}

// relativizeReg subtracts base from the even-indexed (address) cells of a reg
// value, mirroring device_tree.clj relative-reg. The value is the loosely-typed
// dt-prop value (typically a []any of numbers).
func relativizeReg(v any, base uint64) any {
	xs, ok := v.([]any)
	if !ok {
		return v
	}
	out := make([]any, len(xs))
	for i, e := range xs {
		n, ok := toUint64(e)
		switch {
		case ok && i%2 == 0:
			out[i] = n - base
		case ok:
			out[i] = n
		default:
			out[i] = e
		}
	}
	return out
}

// dtProps renders the merged dt-props (already device-over-class merged) as dts
// properties sorted alphabetically by key, faithful to the Clojure `sort` call
// in device_tree.clj. A "reg" key is relativized against busBase; all other
// keys pass through dtProp unchanged.
func dtProps(props design.DtProps, busBase uint64) []*dts.Prop {
	// Sort by key to match Clojure's (sort dt-props) behavior.
	sorted := make(design.DtProps, len(props))
	copy(sorted, props)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Key < sorted[j].Key })
	out := make([]*dts.Prop, 0, len(sorted))
	for _, kv := range sorted {
		v := kv.Val
		if kv.Key == "reg" {
			v = relativizeReg(v, busBase)
		}
		out = append(out, dtProp(kv.Key, v))
	}
	return out
}

// dtChildren converts the typed dt-children into dts child nodes, preserving the
// source order of each child's properties and recursing into nested children.
func dtChildren(children []design.DtChild) []*dts.Node {
	out := make([]*dts.Node, 0, len(children))
	for _, c := range children {
		node := &dts.Node{Name: c.Name}
		for _, kv := range c.Props {
			node.Props = append(node.Props, dtProp(kv.Key, kv.Val))
		}
		node.Children = dtChildren(c.Children)
		out = append(out, node)
	}
	return out
}

// mergeDtProps merges class dt-props with device dt-props, faithful to the
// Clojure (merge cls-props dev-props): class order is preserved, a device key
// that already exists overrides its value in place, and device-only keys are
// appended. Returns a fresh ordered DtProps.
//
// Note: dtProps re-sorts the merged result for device-node emission, so the
// preserved order here is the data-model representation, not the DTS output
// order. (dt-child props, by contrast, are emitted in their preserved order.)
func mergeDtProps(cls *design.DeviceClass, dev *design.Device) design.DtProps {
	out := make(design.DtProps, len(cls.DtProps))
	copy(out, cls.DtProps)
	for _, kv := range dev.DtProps {
		idx := -1
		for i := range out {
			if out[i].Key == kv.Key {
				idx = i
				break
			}
		}
		if idx >= 0 {
			out[idx].Val = kv.Val
		} else {
			out = append(out, kv)
		}
	}
	return out
}

// devToDT builds the device-tree node for one memory-mapped device, faithful to
// device_tree.clj dev-to-dt. Properties are: the sorted merged dt-props (a reg
// among them relativized to busBase), then — only when dt-props has no explicit
// reg — a default hex reg = <base regWidth> with a "<lo>-<hi>" comment, then —
// when hasIRQ and the device is not a cache controller — interrupts = <vector>.
// children are the class dt-children followed by the device dt-children.
//
// busBase is the soc bus base address (the minimum device base). vector is the
// device's effective IRQ number; hasIRQ reports whether an interrupts property
// should be emitted (the caller resolves dt? on the device's irq set).
// regWidth = 1 << (cls.LeftAddrBit+1); base = uint64(*dev.BaseAddr) - busBase.
func devToDT(dev *design.Device, cls *design.DeviceClass, dtNodeName string, busBase uint64, vector int, hasIRQ bool) *dts.Node {
	merged := mergeDtProps(cls, dev)

	props := dtProps(merged, busBase)

	if _, hasReg := merged.Get("reg"); !hasReg && dev.BaseAddr != nil {
		// (a data-bus device with no base-addr is an upstream/P4e validation error;
		// guard the deref so emit degrades to a reg-less node rather than panicking.)
		absBase := uint64(*dev.BaseAddr)
		base := absBase - busBase
		regWidth := uint64(1) << (cls.LeftAddrBit + 1)
		props = append(props, &dts.Prop{
			Name:   "reg",
			Values: []dts.Value{dts.Cells{Nums: []uint64{base, regWidth}, Hex: true}},
			Cmt:    fmt.Sprintf("%08X-%08X", absBase, absBase+regWidth-1),
		})
	}

	clsCompatV, _ := cls.DtProps.Get("compatible")
	clsCompat, _ := clsCompatV.(string)
	if hasIRQ && clsCompat != cacheCompat {
		props = append(props, &dts.Prop{
			Name:   "interrupts",
			Values: []dts.Value{dts.Cells{Nums: []uint64{uint64(vector)}, Hex: true}},
		})
	}

	// dt-children come from the class only; the design.Device model carries no
	// dt-children field (the Clojure (:dt-children dev) is always nil for the
	// migrated YAML boards).
	kids := dtChildren(cls.DtChildren)

	return &dts.Node{Name: dtNodeName, Props: props, Children: kids}
}
