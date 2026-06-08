package emit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/j-core/jcore-soc/tools/socgen/board"
	"github.com/j-core/jcore-soc/tools/socgen/elaborate"
	"github.com/j-core/jcore-soc/tools/socgen/vhdl"
)

// TestDataBusParityMimasV2 is the headline semantic-parity check: it elaborates
// and emits the real mimas_v2 board, then compares the data-bus substructures of
// our output against the committed golden devices.vhd. Comparisons are normalized
// (lower-case, spaces stripped) so comments/whitespace/case never cause false
// negatives; a mismatch is a real divergence from the Clojure reference.
func TestDataBusParityMimasV2(t *testing.T) {
	root := os.Getenv("JCORE_SOC_ROOT")
	if root == "" {
		t.Skip("JCORE_SOC_ROOT not set")
	}
	b, lerr := board.Load(root, "mimas_v2")
	if b == nil || b.Design == nil {
		t.Fatalf("board.Load: %v", lerr)
	}
	res, rerr := elaborate.Elaborate(b)
	if rerr != nil {
		t.Logf("elaborate notes: %v", rerr)
	}
	out, derr := Devices(res)
	if derr != nil {
		t.Logf("emit notes: %v", derr)
	}
	if !strings.Contains(out, "use work.data_bus_pack.all") {
		t.Errorf("missing use work.data_bus_pack.all clause")
	}
	ours := mustArch(t, out)
	golden := mustArchFile(t, filepath.Join(root, "targets/boards/mimas_v2/devices.vhd"))

	// 1) device_t enum literals match (order + names).
	g, o := enumLits(golden, "device_t"), enumLits(ours, "device_t")
	if len(g) == 0 {
		t.Fatalf("enumLits empty for golden device_t — parser/golden problem")
	}
	if !eqStrs(g, o) {
		t.Errorf("device_t enum mismatch:\n golden=%v\n ours  =%v", g, o)
	}
	// 2) decode_address leaf returns in order (each leaf's returned device).
	gr, or := decodeReturns(golden), decodeReturns(ours)
	if len(gr) == 0 {
		t.Fatalf("decodeReturns empty for golden — parser/golden problem")
	}
	if !eqStrs(gr, or) {
		t.Errorf("decode_address returns mismatch:\n golden=%v\n ours  =%v", gr, or)
	}
	// 3) each data-bus device's db wiring: formal -> devs_bus_i/o(DEV_x).
	gw, ow := dbWiring(golden), dbWiring(ours)
	if len(gw) == 0 {
		t.Fatalf("dbWiring empty for golden — parser/golden problem")
	}
	if !eqMapSS(gw, ow) {
		t.Errorf("device db wiring mismatch:\n golden=%v\n ours  =%v", gw, ow)
	}
	// 4) the master mux/loopback statements are present (normalized).
	for _, want := range []string{
		"active_dev<=decode_address(cpu0_periph_dbus_o.a)",
		"cpu0_periph_dbus_i<=devs_bus_i(active_dev)",
		"devs_bus_i(none)<=loopback_bus(devs_bus_o(none))",
		"cpu1_periph_dbus_i<=loopback_bus(cpu1_periph_dbus_o)",
		"devs_bus_o(dev)<=mask_data_o(cpu0_periph_dbus_o,to_bit(dev=active_dev))",
	} {
		if !archHasAssign(ours, want) {
			t.Errorf("missing data-bus statement %q", want)
		}
	}
}

// --- helpers ---------------------------------------------------------------

func mustArch(t *testing.T, src string) *vhdl.ArchitectureBody {
	t.Helper()
	df, err := vhdl.ParseFile(vhdl.NewFileSet(), "devices.vhd", []byte(src))
	if err != nil {
		t.Fatalf("parse failed: %v\n---\n%s", err, src)
	}
	for _, u := range df.Units {
		if a, ok := u.(*vhdl.ArchitectureBody); ok {
			return a
		}
	}
	t.Fatalf("no architecture body found in:\n%s", src)
	return nil
}

func mustArchFile(t *testing.T, path string) *vhdl.ArchitectureBody {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return mustArch(t, string(data))
}

// enumLits returns the lower-cased literals of the enum type named typeName.
func enumLits(arch *vhdl.ArchitectureBody, typeName string) []string {
	for _, d := range arch.Decls {
		td, ok := d.(*vhdl.TypeDecl)
		if !ok || !strings.EqualFold(td.Name, typeName) {
			continue
		}
		if ed, ok := td.Def.(*vhdl.EnumDef); ok {
			out := make([]string, len(ed.Lits))
			for i, l := range ed.Lits {
				out[i] = strings.ToLower(l)
			}
			return out
		}
	}
	return nil
}

// decodeReturns walks the decode_address function body and collects the
// lower-cased device returned by each return statement, in source order.
func decodeReturns(arch *vhdl.ArchitectureBody) []string {
	for _, d := range arch.Decls {
		sb, ok := d.(*vhdl.SubprogramBody)
		if !ok || !strings.EqualFold(sb.Designator, "decode_address") {
			continue
		}
		var out []string
		collectReturns(sb.Stmts, &out)
		return out
	}
	return nil
}

func collectReturns(stmts []vhdl.Stmt, out *[]string) {
	for _, s := range stmts {
		switch st := s.(type) {
		case *vhdl.ReturnStmt:
			if st.Value != nil {
				*out = append(*out, exprStr(st.Value))
			} else {
				*out = append(*out, "")
			}
		case *vhdl.IfStmt:
			collectReturns(st.Then, out)
			for _, ei := range st.Elsifs {
				collectReturns(ei.Stmts, out)
			}
			collectReturns(st.Else, out)
		}
	}
}

// dbWiring records, per data-bus device instantiation, the formal->actual mapping
// for every port whose actual renders to devs_bus_i(...)/devs_bus_o(...).
func dbWiring(arch *vhdl.ArchitectureBody) map[string]string {
	out := map[string]string{}
	for _, s := range arch.Stmts {
		inst, ok := s.(*vhdl.InstantiationStmt)
		if !ok {
			continue
		}
		for _, a := range inst.PortMap {
			act := exprStr(a.Actual)
			if strings.HasPrefix(act, "devs_bus_i(") || strings.HasPrefix(act, "devs_bus_o(") {
				out[strings.ToLower(inst.Label)+"."+strings.ToLower(a.Formal)] = act
			}
		}
	}
	return out
}

// archHasAssign reports whether a top-level (or bus_split inner) concurrent signal
// assignment renders to the given normalized "target<=value" string.
func archHasAssign(arch *vhdl.ArchitectureBody, normalized string) bool {
	var walk func(stmts []vhdl.Stmt) bool
	walk = func(stmts []vhdl.Stmt) bool {
		for _, s := range stmts {
			switch st := s.(type) {
			case *vhdl.ConcurrentSignalAssign:
				if len(st.Waveform) > 0 && exprStr(st.Target)+"<="+exprStr(st.Waveform[0].Value) == normalized {
					return true
				}
			case *vhdl.GenerateStmt:
				if walk(st.Stmts) {
					return true
				}
			}
		}
		return false
	}
	return walk(arch.Stmts)
}

// exprStr renders an expression to a normalized (lower-cased, space-stripped)
// string sufficient for the comparisons above.
func exprStr(e vhdl.Expr) string {
	if e == nil {
		return ""
	}
	switch x := e.(type) {
	case *vhdl.Ident:
		return norm(x.Name)
	case *vhdl.BasicLit:
		return norm(x.Value)
	case *vhdl.ParenExpr:
		return "(" + exprStr(x.X) + ")"
	case *vhdl.BinaryExpr:
		return exprStr(x.X) + norm(x.Op.String()) + exprStr(x.Y)
	case *vhdl.CallExpr:
		var args []string
		for _, a := range x.Args {
			args = append(args, exprStr(a.Actual))
		}
		return exprStr(x.Fun) + "(" + strings.Join(args, ",") + ")"
	default:
		return ""
	}
}

func norm(s string) string { return strings.ToLower(strings.ReplaceAll(s, " ", "")) }

func eqStrs(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func eqMapSS(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}
