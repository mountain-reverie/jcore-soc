#!/usr/bin/env bash
# Simulate the *generated* ULX3S SoC end to end: regenerate the SoC from
# design.yaml (socgen -> devices.vhd/soc.vhd/pad_ring.vhd + ulx3s.lpf) and the
# clock config (make -> output/ulx3s/config/config.vhd), assert the committed
# generated artifacts did not drift, then elaborate the generated pad_ring(impl)
# (-> soc -> devices) as the board top and drive it with ulx3s_gen_tb, checking
# the full M0-M2 feature set (banner, SDRAM, SPI loopback).
#
# pad_ring binds clkgen(ecp5)/EHXPLLL (the real ECP5 PLL primitive); ghdl cannot
# elaborate it, so the sim flow analyzes the sim-only stand-ins
# tb/ehxpll_sim.vhd + ulx3s_clkgen_ecp5.vhd (passthrough PLL) instead. The tb
# feeds clk_25mhz at the post-PLL 20 MHz rate, so the passthrough is exact.
#
# The bootram unit tb uses its own boot_image_pkg (boot_image_pkg_test.vhd);
# the full-system tb uses the real banner boot_image_pkg. Both declare package
# boot_image_pkg, so they MUST be analyzed in separate work libraries.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../../.." && pwd)"
WORK="${WORK:-/tmp/m0work}"
UNITWORK="${UNITWORK:-/tmp/m0unitwork}"
cd "$ROOT"
rm -rf "$WORK" "$UNITWORK"; mkdir -p "$WORK" "$UNITWORK"

# VARIANT selects the socgen design (j2-direct, j4-rom, or the dual-core
# j2-direct-dual/j4-rom-dual variants). j2-direct is the committed default —
# only for that variant do we assert the committed devices/soc/pad_ring trio
# (+ build.mk/ulx3s.lpf) is byte-identical to what socgen still emits (the
# drift check below). Other variants regenerate on demand and are NOT
# committed, so the drift check is skipped for them.
VARIANT="${VARIANT:-j2-direct}"

# 1. regenerate the SoC + clock config from design.yaml (variant-selected) and,
#    for the default j2-direct variant only, prove the committed generated
#    artifacts (the devices/soc/pad_ring trio, build.mk AND the pin-named
#    ulx3s.lpf) are exactly what socgen still emits — design.yaml is the
#    source of truth. cp/diff (not git) so the check is immune to in-container
#    git ownership.
BD=targets/boards/ulx3s
if [ "$VARIANT" = "j2-direct" ]; then
  SNAP="$(mktemp -d)"
  for f in devices.vhd soc.vhd pad_ring.vhd build.mk ulx3s.lpf; do cp "$BD/$f" "$SNAP/$f"; done
fi
make ulx3s TARGET=soc_gen VARIANT="$VARIANT"
make ulx3s TARGET=vhdl_list.txt VARIANT="$VARIANT"
if [ "$VARIANT" = "j2-direct" ]; then
  for f in devices.vhd soc.vhd pad_ring.vhd build.mk ulx3s.lpf; do
    if ! diff -q "$SNAP/$f" "$BD/$f" >/dev/null; then
      echo "ERROR: committed $f drifted from design.yaml; re-run 'make soc_gen BOARDS=ulx3s' and commit." >&2
      diff -u "$SNAP/$f" "$BD/$f" >&2 || true
      exit 1
    fi
  done
  rm -rf "$SNAP"
fi

# 2. boot image
make -C targets/boards/ulx3s/rom all
perl tools/genbootpkg \
    targets/boards/ulx3s/rom/boot.bin \
    4096 \
    > targets/boards/ulx3s/boot_image_pkg.vhd

# 3. generated sources: cpu (decode generate + v2p), uartlite, cache/bus cores
make -C components/cpu/decode generate
( cd components/cpu && for f in core/mult core/datapath decode/decode_core; do
    LD_LIBRARY_PATH='' perl ../../tools/v2p < "$f.vhm" > "$f.vhd"; done )
LD_LIBRARY_PATH='' perl tools/v2p < components/uartlite/uart.vhm > components/uartlite/uart.vhd
for f in components/cpu/cache/dcache_ccl components/cpu/cache/dcache_mcl \
         components/cpu/cache/icache_ccl components/cpu/cache/icache_mcl \
         components/misc/bus_mux_typecsub components/misc/bus_mux_typec \
         components/misc/gpio2 components/misc/spi2; do
  LD_LIBRARY_PATH='' perl tools/v2p < "$f.vhm" > "$f.vhd"
done
LD_LIBRARY_PATH='' perl tools/v2p < targets/cpumreg.vhm > targets/cpumreg.vhd
LD_LIBRARY_PATH='' perl tools/v2p < components/misc/multi_master_bus_mux.vhm > components/misc/multi_master_bus_mux.vhd
source targets/boards/ulx3s/gen_synth_sources.sh

# 4. full design analyze + run the self-checking banner testbench.
echo "=== ulx3s_gen_tb (generated pad_ring SoC) ==="
GHDL="ghdl -a --std=93 -fexplicit -fsynopsys --workdir=$WORK"
# generated config + clk_config FIRST (the FILES list references work.config /
# work.clk_config; output/ulx3s/config/config.vhd is the real generated package).
$GHDL output/ulx3s/config/config.vhd targets/clk_config.vhd
source targets/boards/ulx3s/filelist.sh   # defines FILES=( ... ) incl. the trio
$GHDL "${FILES[@]}"
# the ECP5 clkgen arch pad_ring binds + its sim-only EHXPLLL stand-in (analyzed
# after FILES so the clkgen entity from ulx3s_clkgen.vhd already exists).
$GHDL targets/boards/ulx3s/ulx3s_clkgen_ecp5.vhd targets/boards/ulx3s/tb/ehxpll_sim.vhd
# sim-only SDRAM behavioral model (deliberately excluded from FILES so synth
# never sees it); must be analyzed before the tb that instantiates it.
$GHDL components/sdram/sdram_model.vhd
$GHDL targets/boards/ulx3s/tb/ulx3s_gen_tb.vhd
ghdl -e --std=93 -fexplicit -fsynopsys --syn-binding --workdir="$WORK" ulx3s_gen_tb
ghdl -r --std=93 -fexplicit -fsynopsys --syn-binding --workdir="$WORK" ulx3s_gen_tb --stop-time=20ms --assert-level=error

# 5. bootram unit testbench (separate work lib: uses the deadbeef test image)
echo "=== bootram_infer_tb ==="
UG="ghdl -a --std=93 -fexplicit -fsynopsys --workdir=$UNITWORK"
$UG components/cpu/cpu2j0_pkg.vhd \
   targets/boards/ulx3s/tb/boot_image_pkg_test.vhd \
   components/memory/bootram_infer.vhd \
   targets/boards/ulx3s/tb/bootram_infer_tb.vhd
ghdl -e --std=93 -fexplicit -fsynopsys --syn-binding --workdir="$UNITWORK" bootram_infer_tb
ghdl -r --std=93 -fexplicit -fsynopsys --syn-binding --workdir="$UNITWORK" bootram_infer_tb --stop-time=20us --assert-level=error
