#!/usr/bin/env bash
# Generate synth-clean VHDL copies for the ghdl-yosys (ECP5) flow. Sourced/run by
# both sim.sh and synth.sh so sim and synth share one code path. Run from the
# repo root. Outputs land in targets/boards/ulx3s/generated/ (gitignored).
#
#  - ddr_ram_mux: strip the soc_gen-only `group` + `soc_port_global_name`
#    metadata that ghdl --synth asserts on (synth-vhdl_decls). The ULX3S top is
#    hand-written; no soc_gen consumes that metadata, so removing it is inert.
#
#  - icache/dcache: rewrite the intentional transparent latches (the "0.5 cycle
#    delay" CDC elements) as falling-edge flip-flops. Each latch samples a
#    rising-edge register (thisb*_r), which is stable through the clock-low
#    phase, so a negedge FF is the exact same half-cycle delay -- but it is not a
#    combinational loop. The latch form routes to a LUT-with-feedback that
#    nextpnr's timing analyzer rejects ("combinational loops"). The cache files
#    live in the cpu submodule; generating copies here keeps the submodule clean.
#
# Simulation behaves identically (the latch input only changes on the rising
# edge), so both flows use these copies.
set -euo pipefail
GEN=targets/boards/ulx3s/generated
mkdir -p "$GEN"

# ddr_ram_mux: drop the translate_off..translate_on group block and the
# soc_port_global_name attribute specifications.
perl -0pe 's/-- synopsys translate_off.*?-- synopsys translate_on\n//s;
           s/^\s*attribute soc_port_global_name of .*?;\n//mg' \
  targets/ddr_ram_mux/ddr_ram_mux.vhd > "$GEN/ddr_ram_mux.vhd"

# icache/dcache: transparent latch -> falling-edge FF. \x27 is a single quote so
# the whole perl program can stay in bash single quotes.
for f in icache dcache; do
  perl -0pe 's/(\w+\s*:\s*)process\((clk\d+),\s*this\w+_r\)\s*--\s*transparent latch 0\.5 cycle delay\s*\n\s*begin\s*\n\s*if\s+\2\s*=\s*\x270\x27\s*then\n(.*?\n)\s*end if;\s*\n\s*end process;/${1}process(${2}) -- 0.5 cycle delay (transparent latch -> negedge FF for synth)\n  begin\n    if falling_edge(${2}) then\n${3}    end if;\n  end process;/sg' \
    "components/cpu/cache/$f.vhd" > "$GEN/$f.vhd"
done
