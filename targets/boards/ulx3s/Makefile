# ULX3S (Lattice ECP5 LFE5U-85F) — board registration for socgen.
#
# The FPGA build flow (ghdl + yosys + nextpnr-ecp5 + ecppack) lives in this
# board's synth.sh / sim.sh, not here. This Makefile exists only so socgen's
# `make <board> TARGET=vhdl_list.txt` can assemble the VHDL interface library
# and generate the SoC. Being ECP5, it does NOT include xilinx.mk.

all:
	@echo "ULX3S synthesis/simulation is driven by synth.sh / sim.sh"
	@echo "(ghdl + yosys + nextpnr-ecp5). This Makefile is for socgen only."

# ULX3S CPU/mem clock = 20 MHz (period 50 ns) from the EHXPLLL. The generated
# config derives CFG_CLK_CPU_FREQ_HZ = PLLE2_HZ / PERIOD_NS, and PERIOD_NS =
# CLK_CPU_DIVIDE; so divide=50 => 1e9/50 = 20 MHz. Single clock domain (mem=cpu).
CONFIG_CLK_CPU_DIVIDE := 50
CONFIG_CLK_MEM_2X_DIVIDE := 25

include $(TOP_DIR)/soc_config.mk
include $(TOOLS_DIR)/soc.mk

# Board-specific entities that design.yaml references but that are not part of
# the global FPGA VHDL set: the single-CPU M0 `cpus` configuration, the clkgen
# wrapper, and the reset synchronizer. Append so they land in vhdl_list.txt.
VHDL_FILES += targets/boards/ulx3s/cpus_one_m0.vhd
VHDL_FILES += targets/boards/ulx3s/ulx3s_clkgen.vhd
VHDL_FILES += targets/boards/ulx3s/reset_sync.vhd
VHDL_FILES += targets/boards/ulx3s/aic_irq_gen.vhd
# SDRAM controller + ASIC-portable IO cells (the BufEntity inout pad target).
VHDL_FILES += components/sdram/sdram_pkg.vhd
VHDL_FILES += components/sdram/sdram_ctrl.vhd
VHDL_FILES += components/sdram/sdram_iocells.vhd

project = $(BOARD_NAME)
top_module = pad_ring
