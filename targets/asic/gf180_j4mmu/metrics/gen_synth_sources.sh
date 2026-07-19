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
