# targets/asic/gf180_j4mmu/Makefile -- soc_gen board registration for the GF180
# ASIC target, mirroring targets/boards/ulx3s/Makefile minus the FPGA
# bitstream rules (no synth.sh/sim.sh, no pad_ring/clkgen/lpf). This Makefile
# exists only so soc_gen's `make <board> TARGET=vhdl_list.txt` can assemble
# the VHDL interface library and generate the SoC.
#
# targets/boards/gf180_j4mmu is a symlink to this directory: the top-level
# Makefile's board discovery and soc_gen's board.Load both hardcode
# targets/boards/<name>, so the symlink lets this ASIC target participate in
# the existing board machinery without a board name/path collision.

all:
	@echo "gf180_j4mmu has no FPGA/ASIC backend flow yet. This Makefile is for soc_gen only."

# Clock config placeholders (not yet tied to a real GF180 PLL/clock source --
# this target has no clkgen; clk/reset are primary ports of the logical `soc`
# top). Kept for CFG_CLK_CPU_FREQ_HZ used by uart0/spi2 baud/clock generics.
CONFIG_CLK_CPU_DIVIDE := 50
CONFIG_CLK_MEM_2X_DIVIDE := 25

# Only one variant exists today (see design.yaml: j4-rom topology).
VARIANT ?=

include $(TOP_DIR)/soc_config.mk
include $(TOOLS_DIR)/soc.mk

# Board-specific entities that design.yaml references but that are not part of
# the global FPGA VHDL set: the single-CPU M0 `cpus` configuration and the AIC
# IRQ generator/combiner. Reused verbatim from targets/boards/ulx3s (generic,
# not ECP5-specific) rather than duplicated.
VHDL_FILES += targets/boards/ulx3s/cpus_one_m0_arch.vhd
VHDL_FILES += targets/boards/ulx3s/aic_irq_gen.vhd
VHDL_FILES += targets/boards/ulx3s/aic_irq_combine.vhd
# SDRAM controller (used directly by the logical `soc` top; no ASIC IO-cell
# wrapper needed since there is no pad_ring for this target).
VHDL_FILES += components/sdram/sdram_pkg.vhd
VHDL_FILES += components/sdram/sdram_ctrl.vhd

project = $(BOARD_NAME)
top_module = soc
