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

func TestDevicesEntityPorts(t *testing.T) {
	res := &elaborate.Resolution{
		Signals: map[string]*elaborate.Signal{
			"clk_sys":      {Name: "clk_sys", Type: &elaborate.ResolvedType{Mark: "std_logic"}},
			"flash_cs":     {Name: "flash_cs", Type: &elaborate.ResolvedType{Mark: "std_logic_vector", Left: iptr(1), Right: iptr(0), Dir: "downto"}},
			"cpu0_event_o": {Name: "cpu0_event_o", Type: &elaborate.ResolvedType{Mark: "cpu_event_o_t"}},
		},
		SignalLocations: &elaborate.SignalLocations{
			TopDevices: []elaborate.PortLoc{
				{Name: "clk_sys", Dir: "in"},
				{Name: "cpu0_event_o", Dir: "in"},
				{Name: "flash_cs", Dir: "out"},
			},
		},
	}
	ports := devicesEntityPorts(res)
	if len(ports) != 3 {
		t.Fatalf("want 3 ports, got %d", len(ports))
	}
	got := map[string]string{} // name -> "mode subtype"
	for _, p := range ports {
		got[p.Names[0]] = p.Mode + " " + p.SubtypeMark
	}
	if got["clk_sys"] != "in std_logic" {
		t.Errorf("clk_sys = %q", got["clk_sys"])
	}
	if got["cpu0_event_o"] != "in cpu_event_o_t" {
		t.Errorf("cpu0_event_o = %q", got["cpu0_event_o"])
	}
	if got["flash_cs"] != "out std_logic_vector" { // the (1 downto 0) constraint is carried separately in Constraint
		t.Errorf("flash_cs = %q", got["flash_cs"])
	}
}

func TestDevicesReworkStructure(t *testing.T) {
	res := &elaborate.Resolution{
		Classes: map[string]*elaborate.ResolvedClass{
			"gpioc": {Entity: &iface.Entity{Name: "pio"}, ArchName: "beh"},
		},
		Devices: []*elaborate.ResolvedDevice{
			{Name: "gpio", Class: "gpioc", Ports: []*elaborate.ResolvedPort{
				{Name: "clk", Kind: elaborate.KindSignal, GlobalSignal: "clk_sys"},
				{Name: "d", Kind: elaborate.KindSignal, GlobalSignal: "gpio_internal"},
			}},
		},
		TopEntities: map[string]*elaborate.ResolvedEntity{ // must NOT be instantiated in devices.vhd
			"cpus": {Name: "cpus", Entity: &iface.Entity{Name: "cpus"}, ArchName: "rtl"},
		},
		Signals: map[string]*elaborate.Signal{
			"clk_sys":       {Name: "clk_sys", Type: &elaborate.ResolvedType{Mark: "std_logic"}},
			"gpio_internal": {Name: "gpio_internal", Type: &elaborate.ResolvedType{Mark: "std_logic"}},
		},
		SignalLocations: &elaborate.SignalLocations{
			TopDevices: []elaborate.PortLoc{{Name: "clk_sys", Dir: "in"}},
			Devices:    []string{"gpio_internal"},
		},
	}
	out, err := Devices(res)
	if err != nil {
		t.Fatalf("Devices: %v", err)
	}
	df, perr := vhdl.ParseFile(vhdl.NewFileSet(), "devices.vhd", []byte(out))
	if perr != nil {
		t.Fatalf("re-parse: %v\n%s", perr, out)
	}
	var ent *vhdl.EntityDecl
	var arch *vhdl.ArchitectureBody
	for _, u := range df.Units {
		switch n := u.(type) {
		case *vhdl.EntityDecl:
			ent = n
		case *vhdl.ArchitectureBody:
			arch = n
		}
	}
	if ent == nil || len(ent.Ports) != 1 || ent.Ports[0].Names[0] != "clk_sys" {
		t.Fatalf("entity ports = %+v", ent)
	}
	declNames := map[string]bool{}
	for _, d := range arch.Decls {
		if sd, ok := d.(*vhdl.SignalDecl); ok {
			declNames[sd.Names[0]] = true
		}
	}
	if !declNames["gpio_internal"] {
		t.Errorf("expected gpio_internal declared; decls=%v", declNames)
	}
	if declNames["clk_sys"] {
		t.Errorf("clk_sys is an entity port and must NOT be declared as a signal")
	}
	labels := map[string]bool{}
	for _, s := range arch.Stmts {
		if i, ok := s.(*vhdl.InstantiationStmt); ok {
			labels[i.Label] = true
		}
	}
	if !labels["gpio"] {
		t.Errorf("gpio device must be instantiated; labels=%v", labels)
	}
	if labels["cpus"] {
		t.Errorf("top-entity cpus must NOT be instantiated in devices.vhd")
	}
}

func TestDevicesExtraAlias(t *testing.T) {
	res := &elaborate.Resolution{
		Classes: map[string]*elaborate.ResolvedClass{"c": {Entity: &iface.Entity{Name: "e"}, ArchName: "a"}},
		Devices: []*elaborate.ResolvedDevice{
			{Name: "d0", Class: "c", Ports: []*elaborate.ResolvedPort{{Name: "o", Kind: elaborate.KindSignal, GlobalSignal: "bidir"}}},
			{Name: "d1", Class: "c", Ports: []*elaborate.ResolvedPort{{Name: "i", Kind: elaborate.KindSignal, GlobalSignal: "bidir"}}},
		},
		Signals: map[string]*elaborate.Signal{"bidir": {Name: "bidir", Type: &elaborate.ResolvedType{Mark: "std_logic"}}},
		SignalLocations: &elaborate.SignalLocations{
			TopDevices:   []elaborate.PortLoc{{Name: "bidir", Dir: "out"}},
			DevicesExtra: map[string]string{"bidir": "sig_bidir"},
		},
	}
	out, err := Devices(res)
	if err != nil {
		t.Fatalf("Devices: %v", err)
	}
	if _, perr := vhdl.ParseFile(vhdl.NewFileSet(), "devices.vhd", []byte(out)); perr != nil {
		t.Fatalf("re-parse: %v\n%s", perr, out)
	}
	for _, want := range []string{"signal sig_bidir", "bidir <= sig_bidir", "=> sig_bidir"} {
		if !strings.Contains(out, want) {
			t.Errorf("DevicesExtra output missing %q:\n%s", want, out)
		}
	}
	// BOTH the writer (d0.o) and reader (d1.i) port maps must use the alias.
	if n := strings.Count(out, "=> sig_bidir"); n != 2 {
		t.Errorf("expected 2 port maps wired to sig_bidir (writer + reader), got %d:\n%s", n, out)
	}
}

func TestStdLogicLit(t *testing.T) {
	sl := &elaborate.ResolvedType{Mark: "std_logic"}
	slv := &elaborate.ResolvedType{Mark: "std_logic_vector"}
	cases := []struct {
		name string
		typ  *elaborate.ResolvedType
		val  design.Value
		want string // "" means expect nil (fallback)
	}{
		{"sl0", sl, design.Value{Kind: design.KindInt, Int: 0}, "'0'"},
		{"sl1", sl, design.Value{Kind: design.KindInt, Int: 1}, "'1'"},
		{"sl2", sl, design.Value{Kind: design.KindInt, Int: 2}, ""},
		{"slv0", slv, design.Value{Kind: design.KindInt, Int: 0}, ""},
		{"nonint", sl, design.Value{Kind: design.KindStr, Text: "x"}, ""},
		{"niltype", nil, design.Value{Kind: design.KindInt, Int: 0}, ""},
	}
	for _, c := range cases {
		got := stdLogicLit(c.typ, c.val)
		if c.want == "" {
			if got != nil {
				t.Errorf("%s: got %v, want nil", c.name, got)
			}
			continue
		}
		lit, ok := got.(*vhdl.BasicLit)
		if !ok || lit.Kind != vhdl.CHARLIT || lit.Value != c.want {
			t.Errorf("%s: got %#v, want CHARLIT %q", c.name, got, c.want)
		}
	}
}
