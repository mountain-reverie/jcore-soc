package cheader

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/j-core/jcore-soc/tools/socgen/board"
	"github.com/j-core/jcore-soc/tools/socgen/elaborate"
)

func TestBoardHSynthetic(t *testing.T) {
	base1 := uint64(0xabcd0000)
	base2 := uint64(0xabcd00c0)
	res := &elaborate.Resolution{
		Classes: map[string]*elaborate.ResolvedClass{
			"gpio": {Name: "gpio", Regs: []*elaborate.ResolvedReg{
				{Name: "value", Addr: 0, Width: 4, ByteRange: [2]int{0, 3}, Mode: "rw"},
				{Name: "mask", Addr: 1, Width: 4, ByteRange: [2]int{4, 7}, Mode: "rw"},
			}},
			"cache_ctrl": {Name: "cache_ctrl", Regs: nil},
		},
		Devices: []*elaborate.ResolvedDevice{
			{Name: "gpio", Class: "gpio", BaseAddr: &base1},
			{Name: "cache_ctrl", Class: "cache_ctrl", BaseAddr: &base2},
		},
	}
	out, err := BoardH(res)
	if err != nil {
		t.Fatalf("BoardH: %v", err)
	}
	for _, w := range []string{
		"#ifndef BOARD_H",
		"#define BOARD_H",
		"#include <inttypes.h>",
		"#define DRAM_BASE 0x10000000",
		"// Memory mapped peripherals",
		"#define DEVICE_GPIO_ADDR       0xabcd0000",
		"#define DEVICE_CACHE_CTRL_ADDR 0xabcd00c0",
		"struct gpio_regs {",
		"  uint32_t value;",
		"  uint32_t mask;",
		"#define DEVICE_GPIO ((volatile struct gpio_regs *) DEVICE_GPIO_ADDR)",
		"#endif",
	} {
		if !strings.Contains(out, w) {
			t.Errorf("synthetic board.h missing %q\n--- got ---\n%s", w, out)
		}
	}
	// cache_ctrl has no regs -> no struct, no pointer macro.
	if strings.Contains(out, "struct cache_ctrl_regs") {
		t.Errorf("synthetic board.h should not emit a struct for a reg-less class")
	}
}

func TestBoardHMimasV2(t *testing.T) {
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
	out, err := BoardH(res)
	if err != nil {
		t.Fatalf("BoardH: %v", err)
	}
	golden, gerr := os.ReadFile(filepath.Join(root, "targets/boards/mimas_v2/board.h"))
	if gerr != nil {
		t.Fatalf("read golden: %v", gerr)
	}
	if out == string(golden) {
		t.Logf("mimas_v2 board.h: EXACT match")
		return
	}
	// Not byte-exact: that IS a failure (the printer is ours). The fragment checks
	// + diff below pinpoint where; do not let a non-exact output pass silently.
	t.Errorf("board.h is not a byte-exact match of the golden")
	for _, w := range []string{
		"#ifndef BOARD_H",
		"#define DRAM_BASE 0x10000000",
		"#define DEVICE_GPIO_ADDR       0xabcd0000",
		"#define DEVICE_CACHE_CTRL_ADDR 0xabcd00c0",
		"struct aic_regs {",
		"uint32_t clock_period; // read-only",
		"uint32_t ignore0;",
		"struct cache_ctrl_regs {",
		"uint32_t ignore0[10];",
		"struct spi_regs {",
		"uint32_t ctrl; // only byte 3",
		"#define DEVICE_GPIO ((volatile struct gpio_regs *) DEVICE_GPIO_ADDR)",
		"#define DEVICE_UART0 ((volatile struct uartlite_regs *) DEVICE_UART0_ADDR)",
		"#endif",
	} {
		if !strings.Contains(out, w) {
			t.Errorf("board.h missing %q", w)
		}
	}
	t.Logf("board.h NOT exact; diff:\n%s", firstDiff(out, string(golden)))
}

func firstDiff(a, c string) string {
	la, lc := strings.Split(a, "\n"), strings.Split(c, "\n")
	for i := 0; i < len(la) && i < len(lc); i++ {
		if la[i] != lc[i] {
			return "L" + strconv.Itoa(i) + " GOT:  " + la[i] + "\nL" + strconv.Itoa(i) + " WANT: " + lc[i]
		}
	}
	return "(length-only diff)"
}
