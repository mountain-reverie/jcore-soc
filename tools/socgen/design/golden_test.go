package design

import (
	"os"
	"path/filepath"
	"testing"
)

// boardsDir returns <JCORE_SOC_ROOT>/targets/boards, skipping the test if the
// root env var is not set (the Makefile sets it). Golden tests load the real
// migrated board specs and assert concrete facts copied from the EDN sources.
func boardsDir(t *testing.T) string {
	t.Helper()
	root := os.Getenv("JCORE_SOC_ROOT")
	if root == "" {
		t.Skip("JCORE_SOC_ROOT not set; run via `make -C tools/socgen test`")
	}
	return filepath.Join(root, "targets", "boards")
}

// loadBoard loads <board>/design.yaml, skipping if it does not yet exist (so the
// suite stays green during incremental migration) and failing on load errors.
func loadBoard(t *testing.T, board string) *Design {
	t.Helper()
	p := filepath.Join(boardsDir(t), board, "design.yaml")
	if _, err := os.Stat(p); err != nil {
		t.Skipf("%s not migrated yet", p)
	}
	d, errs := Load(p)
	if len(errs) != 0 {
		t.Fatalf("Load(%s) errors: %v", p, errs)
	}
	return d
}

func TestGoldenMimasV2(t *testing.T) {
	d := loadBoard(t, "mimas_v2")

	if d.Target != "spartan6" {
		t.Errorf("target = %q, want spartan6", d.Target)
	}
	// device-classes include resolved.
	if len(d.DeviceClasses) == 0 {
		t.Fatal("device-classes did not resolve (empty)")
	}
	if c := d.DeviceClasses["uartlite"]; c == nil || c.Entity != "uartlitedb" {
		t.Errorf("uartlite class = %+v, want entity uartlitedb", c)
	}
	if c := d.DeviceClasses["emac"]; c == nil || c.Configuration != "eth_mac_rmii_fpga" {
		t.Errorf("emac class = %+v, want configuration eth_mac_rmii_fpga", c)
	}
	// 5 devices: gpio, aic0 (via include), spi flash, uart0, cache_ctrl.
	if len(d.Devices) != 5 {
		t.Fatalf("len(devices) = %d, want 5", len(d.Devices))
	}
	// gpio device.
	gpio := d.Devices[0]
	if gpio.Class != "gpio" || gpio.BaseAddr == nil || uint64(*gpio.BaseAddr) != 0xabcd0000 {
		t.Errorf("gpio = %+v", gpio)
	}
	if gpio.IRQ == nil || gpio.IRQ.Int == nil || *gpio.IRQ.Int != 4 {
		t.Errorf("gpio irq = %+v", gpio.IRQ)
	}
	// aic0 merged in from the include + override (cpu/dt-label/ports from include).
	aic0 := d.Devices[1]
	if aic0.Class != "aic" || aic0.Name != "aic0" {
		t.Fatalf("device[1] = %+v, want included aic0", aic0)
	}
	if aic0.CPU == nil || *aic0.CPU != 0 || aic0.DtLabel != "aic" {
		t.Errorf("aic0 cpu/dt-label = %+v / %q", aic0.CPU, aic0.DtLabel)
	}
	if aic0.BaseAddr == nil || uint64(*aic0.BaseAddr) != 0xabcd0200 {
		t.Errorf("aic0 base-addr = %v", aic0.BaseAddr)
	}
	if v := aic0.Generics["c_busperiod"]; v.Kind != KindExpr || v.Text != "CFG_CLK_CPU_PERIOD_NS" {
		t.Errorf("aic0 c_busperiod = %+v", v)
	}
	if v := aic0.Ports["bstb_i"]; v.Kind != KindExpr || v.Text != "cpu0_data_master_en" {
		t.Errorf("aic0 bstb_i = %+v", v)
	}
	// uart0 typed generics + dt-props.
	uart := d.Devices[3]
	if uart.Name != "uart0" || !uart.DtStdout {
		t.Errorf("uart0 = %+v", uart)
	}
	if v := uart.Generics["bps"]; v.Kind != KindFloat || v.Float != 19200 {
		t.Errorf("uart0 bps = %+v", v)
	}
	if v := uart.Generics["intcfg"]; v.Kind != KindInt || v.Int != 1 {
		t.Errorf("uart0 intcfg = %+v", v)
	}
	// cache_ctrl uses the map-form irq.
	cc := d.Devices[4]
	if cc.Class != "cache_ctrl" || cc.IRQ == nil || cc.IRQ.Named == nil {
		t.Fatalf("cache_ctrl irq = %+v", cc.IRQ)
	}
	if e := cc.IRQ.Named["int0"]; e == nil || e.CPU != 0 || e.IRQ != 3 {
		t.Errorf("cache_ctrl int0 = %+v", e)
	}
	if e := cc.IRQ.Named["int1"]; e == nil || e.CPU != 1 || e.DT == nil || *e.DT != false {
		t.Errorf("cache_ctrl int1 = %+v", e)
	}
	// the freq_to_read_sample_tm(...) generic survives as verbatim Expr.
	dc := d.TopEntities["ddr_ctrl"]
	if dc == nil {
		t.Fatal("top-entity ddr_ctrl missing")
	}
	if v := dc.Generics["READ_SAMPLE_TM"]; v.Kind != KindExpr || v.Text != "freq_to_read_sample_tm(CFG_CLK_MEM_FREQ_HZ)" {
		t.Errorf("ddr_ctrl READ_SAMPLE_TM = %+v", v)
	}
}

func TestGoldenTurtle1v0(t *testing.T) {
	d := loadBoard(t, "turtle_1v0")

	if d.Target != "spartan6" {
		t.Errorf("target = %q", d.Target)
	}
	// device-classes include + emac override merged.
	emac := d.DeviceClasses["emac"]
	if emac == nil {
		t.Fatal("emac class missing")
	}
	if v := emac.Generics["ASYNC_BRIDGE_IMPL2"]; v.Kind != KindBool || v.Bool != true {
		t.Errorf("emac ASYNC_BRIDGE_IMPL2 = %+v, want override true", v)
	}
	if v := emac.Generics["INSERT_WRITE_DELAY_ETHRX"]; v.Kind != KindBool || v.Bool != true {
		t.Errorf("emac INSERT_WRITE_DELAY_ETHRX = %+v (from base)", v)
	}
	if p := emac.Ports["clk_emac"]; p.Kind != KindExpr || p.Text != "clk_emac" {
		t.Errorf("emac clk_emac port = %+v", p)
	}
	// 7 devices: gpio, aic0, aic1, spi, uart0, emac, cache_ctrl.
	if len(d.Devices) != 7 {
		t.Fatalf("len(devices) = %d, want 7", len(d.Devices))
	}
	// aic0 from include with an override generic (rtc_sec_length34b true).
	aic0 := d.Devices[1]
	if aic0.Name != "aic0" {
		t.Fatalf("device[1] = %+v", aic0)
	}
	if v := aic0.Generics["rtc_sec_length34b"]; v.Kind != KindBool || v.Bool != true {
		t.Errorf("aic0 rtc_sec_length34b = %+v, want override true", v)
	}
	if v := aic0.Generics["c_busperiod"]; v.Text != "CFG_CLK_CPU_PERIOD_NS" {
		t.Errorf("aic0 c_busperiod = %+v (from base)", v)
	}
	// emac device typed generics.
	emacDev := d.Devices[5]
	if emacDev.Class != "emac" || emacDev.BaseAddr == nil || uint64(*emacDev.BaseAddr) != 0xabce0000 {
		t.Errorf("emac device = %+v", emacDev)
	}
	if v := emacDev.Generics["c_addr_width"]; v.Kind != KindInt || v.Int != 11 {
		t.Errorf("emac c_addr_width = %+v", v)
	}
}

func TestGoldenTurtle1v1(t *testing.T) {
	dir := boardsDir(t)
	p := filepath.Join(dir, "turtle_1v1", "design.yaml")
	if _, err := os.Stat(p); err != nil {
		t.Skipf("%s not migrated yet", p)
	}
	// turtle_1v1 faithfully mirrors its EDN, which includes
	// ../sei_device_classes — a file that does not exist in this repo (the
	// common include is commented out in the EDN, and no sei file was ever
	// committed). The board's device-classes therefore cannot resolve. The
	// YAML mirrors that broken state for P6 parity; until a sei spec exists
	// the load is expected to fail on the missing include, so we skip rather
	// than fabricate device classes.
	if _, err := os.Stat(filepath.Join(dir, "sei_device_classes.yaml")); err != nil {
		d, errs := Load(p)
		if len(errs) == 0 {
			t.Fatalf("expected load to fail on missing sei include, but it succeeded: %+v", d)
		}
		t.Skipf("turtle_1v1 references missing ../sei_device_classes (pre-existing in EDN); load errs: %v", errs)
	}

	d := loadBoard(t, "turtle_1v1")
	if d.Target != "spartan6" {
		t.Errorf("target = %q", d.Target)
	}
	if len(d.Devices) != 8 {
		t.Fatalf("len(devices) = %d, want 8", len(d.Devices))
	}
	// gpsif device (unique to turtle_1v1) with verbatim symbol ports.
	var gps *Device
	for _, dev := range d.Devices {
		if dev.Class == "gpsif" {
			gps = dev
		}
	}
	if gps == nil {
		t.Fatal("gpsif device missing")
	}
	if gps.BaseAddr == nil || uint64(*gps.BaseAddr) != 0xabcc0000 {
		t.Errorf("gpsif base-addr = %v", gps.BaseAddr)
	}
	if gps.IRQ == nil || gps.IRQ.Int == nil || *gps.IRQ.Int != 2 {
		t.Errorf("gpsif irq = %+v", gps.IRQ)
	}
	if v := gps.Ports["bi"]; v.Kind != KindExpr || v.Text != "BIST_SCAN_NOP" {
		t.Errorf("gpsif bi port = %+v", v)
	}
	if v := gps.Generics["GPSIF_NC"]; v.Kind != KindInt || v.Int != 5 {
		t.Errorf("gpsif GPSIF_NC = %+v", v)
	}
}

func TestGoldenMicroboard(t *testing.T) {
	d := loadBoard(t, "microboard")

	if d.Target != "spartan6" {
		t.Errorf("target = %q", d.Target)
	}
	// device-classes include + emac override (entity replaced, configuration removed).
	emac := d.DeviceClasses["emac"]
	if emac == nil {
		t.Fatal("emac class missing")
	}
	if emac.Entity != "eth_mac" {
		t.Errorf("emac entity = %q, want eth_mac (override)", emac.Entity)
	}
	// 5 inline devices: gpio, aic0, uart0, spi flash, emac.
	if len(d.Devices) != 5 {
		t.Fatalf("len(devices) = %d, want 5", len(d.Devices))
	}
	aic0 := d.Devices[1]
	if aic0.Class != "aic" || aic0.Name != "aic0" || aic0.CPU == nil || *aic0.CPU != 0 {
		t.Errorf("aic0 = %+v", aic0)
	}
	if v := aic0.Generics["rtc_sec_length34b"]; v.Kind != KindBool || v.Bool != true {
		t.Errorf("aic0 rtc_sec_length34b = %+v", v)
	}
	uart := d.Devices[2]
	if uart.Name != "uart0" {
		t.Fatalf("device[2] = %+v", uart)
	}
	if v := uart.Generics["bps"]; v.Kind != KindFloat || v.Float != 115200 {
		t.Errorf("uart0 bps = %+v", v)
	}
	// top-entity cpus config and freq generic.
	cpus := d.TopEntities["cpus"]
	if cpus == nil || cpus.Configuration != "one_cpu_nocopro_decode_rom_fpga" {
		t.Errorf("cpus top-entity = %+v", cpus)
	}
}
