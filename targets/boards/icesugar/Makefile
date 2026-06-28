# iCESugar (Lattice iCE40 UP5K) — board registration for socgen.
#
# The FPGA build flow (ghdl + yosys synth_ice40 + nextpnr-ice40 + icepack)
# lives in this board's synth.sh / sim.sh, not here. This Makefile exists only
# so socgen's `make icesugar TARGET=vhdl_list.txt` can assemble the VHDL
# interface library and generate the SoC. Being iCE40, it does NOT include
# xilinx.mk.

all:
	@echo "iCESugar synthesis/simulation is driven by synth.sh / sim.sh"
	@echo "(ghdl + yosys synth_ice40 + nextpnr-ice40). This Makefile is for socgen only."

# iCESugar CPU clock = 12 MHz (period 83 ns) directly from the on-board
# oscillator (no PLL). These CONFIG_* values are not used to generate config.vhd
# here (the board ships a hand-written config.vhd, like ULX3S), but are kept for
# documentation / soc_config.mk completeness.
CONFIG_CLK_CPU_DIVIDE := 83
CONFIG_CLK_MEM_2X_DIVIDE := 42

# Single CPU/cache variant; design.yaml carries the cpu: block directly.
VARIANT ?=

include $(TOP_DIR)/soc_config.mk
include $(TOOLS_DIR)/soc.mk

# Board-specific entities that design.yaml references but that are not part of
# the global FPGA VHDL set: the hand-written EBR-only `cpus` architecture and
# the 12 MHz clkgen wrapper. The clock constants (CFG_CLK_CPU_PERIOD_NS etc.)
# come from the generated config/config.vhd (driven by the CONFIG_* values
# above), so no hand-written board config package is needed.
VHDL_FILES += targets/boards/icesugar/cpus_one_ebr.vhd
VHDL_FILES += targets/boards/icesugar/ice_clkgen.vhd
# On-chip boot RAM (inferred iCE40 EBR): all memory is here, no SDRAM.
VHDL_FILES += components/memory/bootram_infer.vhd

project = $(BOARD_NAME)
top_module = pad_ring
