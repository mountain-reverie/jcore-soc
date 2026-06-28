#!/usr/bin/env bash
# Generate synth-clean VHDL copies for the ghdl-yosys (ECP5) flow. Sourced/run by
# both sim.sh and synth.sh so sim and synth share one code path. Run from the
# repo root. Outputs land in targets/boards/ulx3s/generated/ (gitignored).
#
#  - ddr_ram_mux: strip the soc_gen-only `group` + `soc_port_global_name`
#    metadata that ghdl --synth asserts on (synth-vhdl_decls). The ULX3S top is
#    hand-written; no soc_gen consumes that metadata, so removing it is inert.
#
# (The icache/dcache latch->negedge-FF rewrite that used to live here is gone:
# jcore-cpu now ships the single-clock CDC form directly -- cache_clkmode_sc
# selects POSEDGE _sc phase FFs, comb-loop-free and T/2-free, Part B / PR #36 --
# and the filelist points at cache/{i,d}cache.vhd directly. Only the ddr_ram_mux
# strip remains.)
set -euo pipefail
GEN=targets/boards/ulx3s/generated
mkdir -p "$GEN"

# ddr_ram_mux: drop the translate_off..translate_on group block and the
# soc_port_global_name attribute specifications.
perl -0pe 's/-- synopsys translate_off.*?-- synopsys translate_on\n//s;
           s/^\s*attribute soc_port_global_name of .*?;\n//mg' \
  targets/ddr_ram_mux/ddr_ram_mux.vhd > "$GEN/ddr_ram_mux.vhd"

# cpus_config: the soc_gen-generated `configuration soc_cpus_config of cpus`
# (binds the variant's cpu_synth config). It carries no soc_gen-only metadata,
# so a plain copy into the synth-staging dir suffices; the filelist references
# generated/cpus_config.vhd uniformly with the other staged sources.
cp targets/boards/ulx3s/cpus_config.vhd "$GEN/cpus_config.vhd"

# NOTE: the icache/dcache transparent-latch -> negedge-FF rewrite that used to
# live here is GONE. jcore-cpu now provides the single-clock CDC form directly
# (cache/cache_clkmode_sc.vhd selects POSEDGE _sc phase FFs in cache/{i,d}cache.vhd
# -- comb-loop-free and T/2-free; Part B / PR #36). The filelist points at those
# files directly, so no rewrite/copy is needed.
