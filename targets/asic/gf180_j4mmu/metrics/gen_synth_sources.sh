#!/usr/bin/env bash
# Generate synth-clean VHDL copies for the ghdl-yosys (GF180) generic-synth
# metrics flow, and export GHDL_BASE (the ghdl analyze command string yosys
# consumes). Sourced by synth_gf180.sh. Run from the repo root. Outputs land
# in targets/asic/gf180_j4mmu/metrics/generated/ (gitignored).
#
# Adapted from targets/boards/ulx3s/gen_synth_sources.sh: this target's plain
# `ghdl -a`/`ghdl -e` flow (filelist.sh, README.md) analyzes the raw sources
# fine because it never invokes `ghdl --synth`. The yosys `-m ghdl` plugin
# DOES run ghdl's synth path (`synth -top ...`), which asserts
# (Synth_Attribute_Port) on the same soc_gen-only `soc_port_*` port
# attributes and the ddr_ram_mux `group`/`soc_port_global_name` metadata that
# ulx3s's synth.sh strips for ECP5. Same problem, same fix, applied to this
# target's file set (drops the FPGA-only pad_ring/clkgen/ehxpll entries --
# this target's `soc` has no pad_ring in the first place).
set -euo pipefail
GEN=targets/asic/gf180_j4mmu/metrics/generated
mkdir -p "$GEN"

# ddr_ram_mux: drop the translate_off..translate_on group block and the
# soc_port_global_name attribute specifications (pulled in transitively by
# the `top`/soc macro via the ddr_ram_mux_one_cpu_idcache_fpga configuration).
perl -0pe 's/-- synopsys translate_off.*?-- synopsys translate_on\n//s;
           s/^\s*attribute soc_port_global_name of .*?;\n//mg' \
  targets/ddr_ram_mux/ddr_ram_mux.vhd > "$GEN/ddr_ram_mux.vhd"

# gpio2/uartlitedb/spi2 carry soc_port_irq port attributes (pulled in
# transitively by the `top`/soc macro via `devices`); icache_modereg carries
# soc_port_global_name (dual-core IPI only, not instantiated by this
# single-core target, but stripped too for parity with ulx3s and in case a
# future variant adds it). ghdl --synth cannot handle any of these
# ("unhandled attribute"), so strip every soc_port_* application into
# synth-staging copies analyzed in their place.
for _src in components/misc/gpio2.vhd components/uartlite/uartlitedb.vhd \
            components/misc/spi2.vhd components/cpu/cache/icache_modereg.vhd; do
  perl -0pe 's/^\s*attribute soc_port_\w+ of .*?;\n//mg' \
    "$_src" > "$GEN/$(basename "$_src")"
done

# Canonical analyze order for this target: work/config.vhd + clk_config.vhd
# first (per README.md), then filelist.sh's list with the four synth-clean
# copies above substituted in place of their raw sources. No pad_ring/clkgen/
# ehxpll entries exist in filelist.sh already (this target has no pad_ring).
CFG="output/gf180_j4mmu/config/config.vhd"
if [ ! -f "$CFG" ]; then
  echo "ERROR: $CFG missing -- run 'make gf180_j4mmu TARGET=soc_gen' first" >&2
  exit 1
fi

mapfile -t _RAW_FILES < <(bash targets/asic/gf180_j4mmu/filelist.sh)
FILES=()
for _f in "${_RAW_FILES[@]}"; do
  case "$_f" in
    targets/ddr_ram_mux/ddr_ram_mux.vhd) FILES+=("$GEN/ddr_ram_mux.vhd") ;;
    components/misc/gpio2.vhd)           FILES+=("$GEN/gpio2.vhd") ;;
    components/uartlite/uartlitedb.vhd)  FILES+=("$GEN/uartlitedb.vhd") ;;
    components/misc/spi2.vhd)            FILES+=("$GEN/spi2.vhd") ;;
    components/cpu/cache/icache_modereg.vhd) FILES+=("$GEN/icache_modereg.vhd") ;;
    *) FILES+=("$_f") ;;
  esac
done

WORKDIR="${WORKDIR:-$GEN/work}"
mkdir -p "$WORKDIR"
GHDL_BASE="ghdl --std=93 -fexplicit -fsynopsys --syn-binding --workdir=$WORKDIR $CFG targets/clk_config.vhd ${FILES[*]}"
export GHDL_BASE

# icache_gf180 / dcache_gf180 (synth_gf180.sh's cache_ctrl.*_gf180 special
# cases): full icache_adapter_gf180/dcache_adapter_gf180 -- controller +
# BOTH tag (ram_1rw) and data (ram_2rw) RAM bound to the tech/gf180 vendor
# SRAM macros via lib/memory_tech_lib/tech/gf180/{mem_gf180_config,
# cache_gf180_config}.vhd -- as opposed to the tech/inferred path FILES above
# uses. Built by walking the same FILES list (leaf-first order already
# correct) and substituting at the two divergence points: drop
# tech/inferred/ram_{1,2}rw_infer.vhd for the gf180 memory-macro chain (named
# macro entities, their (sim) archs for the two macros with no gf180 backend,
# the (gf180) archs, the `memories` architectures, and mem_gf180_config.vhd),
# and stop at (drop) cache_config_fpga.vhd -- replaced by
# cache_gf180_config.vhd -- since nothing after that point (ddr_ram_mux /
# devices / soc, which reference the *_fpga configurations by name) is needed
# to elaborate icache_adapter_gf180/dcache_adapter_gf180 standalone.
GF180_MEM_EXTRA=(
  lib/memory_tech_lib/ram_18x2048_1rw.vhd
  lib/memory_tech_lib/ram_32x1x512_2rw.vhd
  lib/memory_tech_lib/ram_2x8x256_1rw.vhd
  lib/memory_tech_lib/ram_2x8x2048_2rw.vhd
  lib/memory_tech_lib/tech/sim/ram_18x2048_1rw_sim.vhd
  lib/memory_tech_lib/tech/sim/ram_32x1x512_2rw_sim.vhd
  lib/memory_tech_lib/tech/gf180/gf180mcu_fd_ip_sram_comp.vhd
  lib/memory_tech_lib/tech/gf180/ram_2x8x256_1rw_gf180.vhd
  lib/memory_tech_lib/tech/gf180/ram_2x8x2048_2rw_gf180.vhd
  lib/memory_tech_lib/ram_1rw_mems.vhd
  lib/memory_tech_lib/ram_2rw_mems.vhd
  lib/memory_tech_lib/tech/gf180/mem_gf180_config.vhd
)
GF180_CACHE_FILES=()
for _f in "${FILES[@]}"; do
  case "$_f" in
    lib/memory_tech_lib/tech/inferred/ram_1rw_infer.vhd) ;;
    lib/memory_tech_lib/tech/inferred/ram_2rw_infer.vhd)
      GF180_CACHE_FILES+=("${GF180_MEM_EXTRA[@]}") ;;
    components/cpu/cache/cache_config_fpga.vhd)
      GF180_CACHE_FILES+=(lib/memory_tech_lib/tech/gf180/cache_gf180_config.vhd)
      break ;;
    *) GF180_CACHE_FILES+=("$_f") ;;
  esac
done
GHDL_BASE_GF180_CACHE="ghdl --std=93 -fexplicit -fsynopsys --syn-binding --workdir=$WORKDIR $CFG targets/clk_config.vhd ${GF180_CACHE_FILES[*]}"
export GHDL_BASE_GF180_CACHE
