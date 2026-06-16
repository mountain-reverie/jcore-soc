#!/usr/bin/env bash
# Build the M0 boot image + cpu sources, analyze the design, and run the
# top-level banner testbench and the bootram unit testbench under ghdl.
#
# The bootram unit tb uses its own boot_image_pkg (boot_image_pkg_test.vhd);
# the full-system tb uses the real banner boot_image_pkg. Both declare package
# boot_image_pkg, so they MUST be analyzed in separate work libraries. The
# full-design analyze runs first (the bootram unit tb's elaborate/run before a
# fresh analyze can confuse ghdl's library state), the bootram unit tb last.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../../.." && pwd)"
WORK="${WORK:-/tmp/m0work}"
UNITWORK="${UNITWORK:-/tmp/m0unitwork}"
cd "$ROOT"
rm -rf "$WORK" "$UNITWORK"; mkdir -p "$WORK" "$UNITWORK"

# 1. boot image
make -C targets/boards/ulx3s/rom
perl tools/genbootpkg targets/boards/ulx3s/rom/main.bin 4096 > targets/boards/ulx3s/boot_image_pkg.vhd

# 2. generated sources: cpu (decode generate + v2p) and uartlite uart.vhd
make -C components/cpu/decode generate
( cd components/cpu && for f in core/mult core/datapath decode/decode_core; do
    LD_LIBRARY_PATH='' perl ../../tools/v2p < "$f.vhm" > "$f.vhd"; done )
LD_LIBRARY_PATH='' perl tools/v2p < components/uartlite/uart.vhm > components/uartlite/uart.vhd
# M1b: v2p the cache + bus-mux cores
for f in components/cpu/cache/dcache_ccl components/cpu/cache/dcache_mcl \
         components/cpu/cache/icache_ccl components/cpu/cache/icache_mcl \
         components/misc/bus_mux_typecsub components/misc/bus_mux_typec; do
  LD_LIBRARY_PATH='' perl tools/v2p < "$f.vhm" > "$f.vhd"
done
# M1b: ghdl --synth asserts (synth-vhdl_decls) on the soc_gen-only `group` +
# `soc_port_global_name` metadata in ddr_ram_mux.vhd. The ULX3S top is
# hand-written (no soc_gen consumes that metadata), so strip it into a
# synth-clean copy used by both sim and synth (sim is unaffected; the metadata
# has no simulation meaning).
perl -0pe 's/-- synopsys translate_off.*?-- synopsys translate_on\n//s;
           s/^\s*attribute soc_port_global_name of .*?;\n//mg' \
  targets/ddr_ram_mux/ddr_ram_mux.vhd > targets/ddr_ram_mux/ddr_ram_mux_synth.vhd

# 3. full design analyze (real banner boot_image_pkg) + banner testbench
echo "=== ulx3s_top_tb ==="
source targets/boards/ulx3s/filelist.sh   # defines FILES=( ... )
GHDL="ghdl -a --std=93 -fexplicit -fsynopsys --workdir=$WORK"
$GHDL "${FILES[@]}"
# sim-only SDRAM behavioral model (deliberately excluded from FILES so synth
# never sees it); must be analyzed before the tb that instantiates it.
$GHDL components/sdram/sdram_model.vhd
$GHDL targets/boards/ulx3s/tb/ulx3s_top_tb.vhd
ghdl -e --std=93 -fexplicit -fsynopsys --syn-binding --workdir="$WORK" ulx3s_top_tb
ghdl -r --std=93 -fexplicit -fsynopsys --syn-binding --workdir="$WORK" ulx3s_top_tb --stop-time=20ms --assert-level=error

# 4. bootram unit testbench (separate work lib: uses the deadbeef test image)
echo "=== bootram_infer_tb ==="
UG="ghdl -a --std=93 -fexplicit -fsynopsys --workdir=$UNITWORK"
$UG components/cpu/cpu2j0_pkg.vhd \
   targets/boards/ulx3s/tb/boot_image_pkg_test.vhd \
   components/memory/bootram_infer.vhd \
   targets/boards/ulx3s/tb/bootram_infer_tb.vhd
ghdl -e --std=93 -fexplicit -fsynopsys --syn-binding --workdir="$UNITWORK" bootram_infer_tb
ghdl -r --std=93 -fexplicit -fsynopsys --syn-binding --workdir="$UNITWORK" bootram_infer_tb --stop-time=20us --assert-level=error
