package design

import (
	"os"
	"path/filepath"
	"testing"
)

func loadString(t *testing.T, src string) (*Design, error) {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "design.yaml")
	if err := os.WriteFile(p, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	return Load(p)
}

func TestLoadFlatDecode(t *testing.T) {
	d, err := loadString(t, `target: spartan6
devices:
  - class: aic
    name: aic0
    base-addr: 0xabcd0200
    irq: 4
    generics:
      c_busperiod: CFG_CLK_CPU_PERIOD_NS
      rtc_sec_length34b: false
      num_cs: 2
      bps: 19.2e3
      label: !str hello
    ports: { bstb_i: cpu0_data_master_en }
zero-signals: [icache0_ctrl, dcache0_ctrl]
`)
	if err != nil {
		t.Fatalf("load errors: %v", err)
	}
	if d.Target != "spartan6" || len(d.Devices) != 1 {
		t.Fatalf("design = %+v", d)
	}
	dev := d.Devices[0]
	if dev.Class != "aic" || dev.Name != "aic0" {
		t.Errorf("device = %+v", dev)
	}
	if dev.BaseAddr == nil || uint64(*dev.BaseAddr) != 0xabcd0200 {
		t.Errorf("base-addr = %v", dev.BaseAddr)
	}
	if dev.IRQ == nil || dev.IRQ.Int == nil || *dev.IRQ.Int != 4 {
		t.Errorf("irq = %v", dev.IRQ)
	}
	// verbatim-by-default + typed values
	g := dev.Generics
	if g["c_busperiod"].Kind != KindExpr || g["c_busperiod"].Text != "CFG_CLK_CPU_PERIOD_NS" {
		t.Errorf("c_busperiod = %+v", g["c_busperiod"])
	}
	if g["rtc_sec_length34b"].Kind != KindBool || g["rtc_sec_length34b"].Bool != false {
		t.Errorf("rtc = %+v", g["rtc_sec_length34b"])
	}
	if g["num_cs"].Kind != KindInt || g["num_cs"].Int != 2 {
		t.Errorf("num_cs = %+v", g["num_cs"])
	}
	if g["bps"].Kind != KindFloat || g["bps"].Float != 19200 {
		t.Errorf("bps = %+v", g["bps"])
	}
	if g["label"].Kind != KindStr || g["label"].Text != "hello" {
		t.Errorf("label = %+v", g["label"])
	}
	if dev.Ports["bstb_i"].Kind != KindExpr || dev.Ports["bstb_i"].Text != "cpu0_data_master_en" {
		t.Errorf("port = %+v", dev.Ports["bstb_i"])
	}
	if len(d.ZeroSignals) != 2 || d.ZeroSignals[0] != "icache0_ctrl" {
		t.Errorf("zero-signals = %v", d.ZeroSignals)
	}
}

func TestLoadBoolCasing(t *testing.T) {
	d, err := loadString(t, `devices:
  - class: c
    generics: { a: TRUE, b: False, c: true, d: FALSE }
`)
	if err != nil {
		t.Fatalf("load errors: %v", err)
	}
	g := d.Devices[0].Generics
	for name, want := range map[string]bool{"a": true, "b": false, "c": true, "d": false} {
		if g[name].Kind != KindBool || g[name].Bool != want {
			t.Errorf("%s = %+v, want KindBool %v", name, g[name], want)
		}
	}
}

func TestLoadMapValuedPort(t *testing.T) {
	// NB: `irq?` is written in block style; YAML flow mappings ({...}) reserve
	// a leading-token `?` as the explicit-key indicator, so `{ irq?: true }`
	// fails to parse. The real specs use block style for these map values too.
	d, err := loadString(t, `devices:
  - class: aic
    ports:
      irq_i:
        irq?: true
      sig: cpu0_sig
`)
	if err != nil {
		t.Fatalf("load errors: %v", err)
	}
	p := d.Devices[0].Ports
	if p["irq_i"].Kind != KindMap {
		t.Fatalf("irq_i kind = %v, want KindMap", p["irq_i"].Kind)
	}
	if p["irq_i"].Map["irq?"] != true {
		t.Errorf("irq_i map = %v", p["irq_i"].Map)
	}
	if p["sig"].Kind != KindExpr || p["sig"].Text != "cpu0_sig" { // scalar still verbatim
		t.Errorf("sig = %+v", p["sig"])
	}
}

// A function-call generic must survive as a verbatim Expr.
func TestLoadFuncCallValue(t *testing.T) {
	d, err := loadString(t, `top-entities:
  ddr_ctrl:
    entity: ddr_fsm
    generics: { READ_SAMPLE_TM: freq_to_read_sample_tm(CFG_CLK_MEM_FREQ_HZ) }
`)
	if err != nil {
		t.Fatalf("load errors: %v", err)
	}
	v := d.TopEntities["ddr_ctrl"].Generics["READ_SAMPLE_TM"]
	if v.Kind != KindExpr || v.Text != "freq_to_read_sample_tm(CFG_CLK_MEM_FREQ_HZ)" {
		t.Errorf("call value = %+v", v)
	}
}

func TestLoadPlugins(t *testing.T) {
	dir := t.TempDir()
	// default_plugins.yaml is !include'd; write both files.
	if err := os.WriteFile(filepath.Join(dir, "default_plugins.yaml"),
		[]byte("- device_tree\n- board.h\n- aic1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "design.yaml"),
		[]byte("plugins: !include default_plugins.yaml\ntarget: spartan6\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	d, err := Load(filepath.Join(dir, "design.yaml"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	want := []string{"device_tree", "board.h", "aic1"}
	if len(d.Plugins) != len(want) {
		t.Fatalf("Plugins = %v, want %v", d.Plugins, want)
	}
	for i := range want {
		if d.Plugins[i] != want[i] {
			t.Errorf("Plugins[%d] = %q, want %q", i, d.Plugins[i], want[i])
		}
	}
}
