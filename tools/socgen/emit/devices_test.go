package emit

import (
	"errors"
	"strings"
	"testing"

	"github.com/j-core/jcore-soc/tools/socgen/design"
	"github.com/j-core/jcore-soc/tools/socgen/elaborate"
	"github.com/j-core/jcore-soc/tools/socgen/iface"
	"github.com/j-core/jcore-soc/tools/socgen/vhdl"
)

func iptr(i int) *int { return &i }

func TestDevicesStructural(t *testing.T) {
	res := &elaborate.Resolution{
		Classes: map[string]*elaborate.ResolvedClass{
			"uartlite": {Entity: &iface.Entity{Name: "uartlitedb"}, ArchName: "rtl"},
		},
		Devices: []*elaborate.ResolvedDevice{
			{Name: "uart0", Class: "uartlite",
				Generics: map[string]design.Value{"width": {Kind: design.KindInt, Int: 8}},
				Ports: []*elaborate.ResolvedPort{
					{Name: "clk", Kind: elaborate.KindSignal, GlobalSignal: "sysclk"},
					{Name: "cs", Kind: elaborate.KindValue, Value: &design.Value{Kind: design.KindExpr, Text: "'1'"}},
					{Name: "irq", Kind: elaborate.KindIRQ},
				}},
		},
		Signals: map[string]*elaborate.Signal{
			"sysclk":   {Name: "sysclk", Type: &elaborate.ResolvedType{Mark: "std_logic"}},
			"flash_cs": {Name: "flash_cs", Type: &elaborate.ResolvedType{Mark: "std_logic_vector", Left: iptr(1), Right: iptr(0), Dir: "downto"}},
		},
	}
	out, err := Devices(res)
	if err != nil {
		t.Fatalf("Devices err: %v", err)
	}
	df, perr := vhdl.ParseFile(vhdl.NewFileSet(), "devices.vhd", []byte(out))
	if perr != nil {
		t.Fatalf("emitted devices.vhd does not re-parse:\n%s\nerr: %v", out, perr)
	}
	var arch *vhdl.ArchitectureBody
	for _, u := range df.Units {
		if a, ok := u.(*vhdl.ArchitectureBody); ok {
			arch = a
		}
	}
	if arch == nil || arch.Name != "impl" || arch.Entity != "devices" {
		t.Fatalf("missing architecture impl of devices: %+v", arch)
	}
	if len(arch.Decls) != 2 {
		t.Errorf("expected 2 signal decls, got %d", len(arch.Decls))
	}
	var inst *vhdl.InstantiationStmt
	for _, s := range arch.Stmts {
		if i, ok := s.(*vhdl.InstantiationStmt); ok && i.Label == "uart0" {
			inst = i
		}
	}
	if inst == nil {
		t.Fatalf("uart0 instantiation missing; emitted:\n%s", out)
	}
	if inst.UnitKind != vhdl.ENTITY || inst.Unit != "work.uartlitedb" || inst.Arch != "rtl" {
		t.Errorf("inst unit = %v %q (%q)", inst.UnitKind, inst.Unit, inst.Arch)
	}
	got := map[string]string{}
	for _, a := range inst.PortMap {
		got[a.Formal] = exprText(a.Actual)
	}
	if got["clk"] != "sysclk" || got["irq"] != "open" {
		t.Errorf("port map = %v", got)
	}
	if len(inst.GenericMap) != 1 || inst.GenericMap[0].Formal != "width" {
		t.Errorf("generic map = %+v", inst.GenericMap)
	}
	// The concrete-vector signal must round-trip its index constraint.
	var flashDecl *vhdl.SignalDecl
	for _, d := range arch.Decls {
		if sd, ok := d.(*vhdl.SignalDecl); ok {
			for _, name := range sd.Names {
				if name == "flash_cs" {
					flashDecl = sd
				}
			}
		}
	}
	if flashDecl == nil || flashDecl.Constraint == nil {
		t.Errorf("flash_cs signal decl missing or has no constraint; emitted:\n%s", out)
	}
	if !strings.Contains(out, "(1 downto 0)") {
		t.Errorf("expected concrete vector constraint (1 downto 0) in output:\n%s", out)
	}
}

func TestDevicesUnboundEntity(t *testing.T) {
	res := &elaborate.Resolution{
		Classes: map[string]*elaborate.ResolvedClass{}, // empty: no class bound
		Devices: []*elaborate.ResolvedDevice{
			{Name: "orphan0", Class: "missing"},
		},
		Signals: map[string]*elaborate.Signal{
			"clk": {Name: "clk", Type: &elaborate.ResolvedType{Mark: "std_logic"}},
		},
	}
	out, err := Devices(res)
	if err == nil || !errors.Is(err, ErrUnboundEntity) {
		t.Fatalf("expected ErrUnboundEntity, got %v", err)
	}
	// best-effort: still emits a parseable file (with the signal, minus the orphan inst).
	if _, perr := vhdl.ParseFile(vhdl.NewFileSet(), "devices.vhd", []byte(out)); perr != nil {
		t.Fatalf("best-effort output should still parse: %v\n%s", perr, out)
	}
}

func exprText(e vhdl.Expr) string {
	switch x := e.(type) {
	case *vhdl.Ident:
		return x.Name
	case *vhdl.BasicLit:
		return x.Value
	default:
		return ""
	}
}

func TestDevicesNil(t *testing.T) {
	out, err := Devices(nil)
	if out != "" || err != nil {
		t.Errorf("Devices(nil) = %q, %v", out, err)
	}
}
