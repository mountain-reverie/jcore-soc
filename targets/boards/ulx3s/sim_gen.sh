#!/usr/bin/env bash
# Simulate the *generated* ULX3S SoC end to end: regenerate the SoC from
# design.yaml (socgen -> devices.vhd/soc.vhd/pad_ring.vhd) and the clock config
# (make -> output/ulx3s/config/config.vhd), assert the committed trio did not
# drift, then drive the generated pad_ring(impl) (-> soc -> devices) with
# ulx3s_gen_tb and check the full M0-M2 feature set (banner, SDRAM, run-from-
# SDRAM, TICK, RTC, GPIO, BTN).
#
# Companion to sim.sh, which sims the hand-written ulx3s_top. Both share the
# generated-source prelude (boot image, cpu, v2p, synth cache copies). This one
# swaps the hand top + its minimal stand-in config for the generated trio, the
# generated config + clk_config, and the padring entities socgen instantiates
# but does not emit (reset_sync, aic_irq_gen).
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../../.." && pwd)"
WORK="${WORK:-/tmp/genwork}"
cd "$ROOT"
rm -rf "$WORK"; mkdir -p "$WORK"

# 1. regenerate the SoC from design.yaml + the clock config, and prove the
#    committed trio is what socgen still emits (design.yaml is the source of
#    truth). socgen also rewrites ulx3s.lpf from the pin rules; the .lpf cutover
#    is deferred (Phase 3), so the hand-written one is preserved and excluded.
#    cp/diff (not git) so the check is immune to in-container git ownership.
BD=targets/boards/ulx3s
SNAP="$(mktemp -d)"
for f in devices.vhd soc.vhd pad_ring.vhd build.mk ulx3s.lpf; do cp "$BD/$f" "$SNAP/$f"; done
make soc_gen BOARDS=ulx3s
cp "$SNAP/ulx3s.lpf" "$BD/ulx3s.lpf"   # restore hand-written .lpf (Phase 3 defers cutover)
make ulx3s TARGET=vhdl_list.txt
for f in devices.vhd soc.vhd pad_ring.vhd build.mk; do
  if ! diff -q "$SNAP/$f" "$BD/$f" >/dev/null; then
    echo "ERROR: committed $f drifted from design.yaml; re-run 'make soc_gen BOARDS=ulx3s' and commit." >&2
    diff -u "$SNAP/$f" "$BD/$f" >&2 || true
    exit 1
  fi
done
rm -rf "$SNAP"

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
source targets/boards/ulx3s/gen_synth_sources.sh

# 4. analyze the generated design + run the self-checking testbench.
echo "=== ulx3s_gen_tb (generated SoC) ==="
source targets/boards/ulx3s/filelist.sh   # defines FILES=( ... )
GHDL="ghdl -a --std=93 -fexplicit -fsynopsys --workdir=$WORK"
# FILES drives the hand-written top; the generated flow uses the socgen trio +
# the generated config instead. Drop the hand top and the minimal stand-in
# config (output/ulx3s/config/config.vhd is the real generated config package).
GEN_FILES=()
for f in "${FILES[@]}"; do
  case "$f" in
    targets/boards/ulx3s/ulx3s_top.vhd|targets/boards/ulx3s/config.vhd) continue ;;
  esac
  GEN_FILES+=("$f")
done
$GHDL output/ulx3s/config/config.vhd targets/clk_config.vhd
$GHDL "${GEN_FILES[@]}"
# padring entities socgen instantiates but does not emit.
$GHDL targets/boards/ulx3s/reset_sync.vhd targets/boards/ulx3s/aic_irq_gen.vhd
# the generated trio, leaf-first (devices <- soc <- pad_ring).
$GHDL targets/boards/ulx3s/devices.vhd targets/boards/ulx3s/soc.vhd targets/boards/ulx3s/pad_ring.vhd
# sim-only SDRAM behavioral model (excluded from synth), then the tb.
$GHDL components/sdram/sdram_model.vhd
$GHDL targets/boards/ulx3s/tb/ulx3s_gen_tb.vhd
ghdl -e --std=93 -fexplicit -fsynopsys --syn-binding --workdir="$WORK" ulx3s_gen_tb
ghdl -r --std=93 -fexplicit -fsynopsys --syn-binding --workdir="$WORK" ulx3s_gen_tb --stop-time=20ms --assert-level=error
