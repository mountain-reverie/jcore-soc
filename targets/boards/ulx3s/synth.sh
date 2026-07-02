#!/usr/bin/env bash
# Build the ULX3S bitstream: ghdl+yosys synth_ecp5 -> nextpnr-ecp5 -> ecppack,
# emit dashboard metrics, and GATE on declared-clock timing closure.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../../.." && pwd)"
WORK="${WORK:-/tmp/m0synth}"
OUT="${OUT:-$ROOT/targets/boards/ulx3s/build}"
cd "$ROOT"; rm -rf "$WORK" "$OUT"; mkdir -p "$WORK" "$OUT"

# 1. boot image
make -C targets/boards/ulx3s/rom all
perl tools/genbootpkg \
    targets/boards/ulx3s/rom/boot.bin \
    4096 \
    > targets/boards/ulx3s/boot_image_pkg.vhd

# 2. generated sources: cpu (decode generate + v2p) and uartlite uart.vhd
make -C components/cpu/decode generate
( cd components/cpu && for f in core/mult core/datapath decode/decode_core; do
    LD_LIBRARY_PATH='' perl ../../tools/v2p < "$f.vhm" > "$f.vhd"; done )
LD_LIBRARY_PATH='' perl tools/v2p < components/uartlite/uart.vhm > components/uartlite/uart.vhd
# M1b: v2p the cache + bus-mux cores
for f in components/cpu/cache/dcache_ccl components/cpu/cache/dcache_mcl \
         components/cpu/cache/icache_ccl components/cpu/cache/icache_mcl \
         components/misc/bus_mux_typecsub components/misc/bus_mux_typec \
         components/misc/gpio2 components/misc/spi2; do
  LD_LIBRARY_PATH='' perl tools/v2p < "$f.vhm" > "$f.vhd"
done
# M1b: generate the synth-clean ddr_ram_mux + cache copies (see the script).
source targets/boards/ulx3s/gen_synth_sources.sh

# 2b. generated clock config (output/ulx3s/config/config.vhd): the FILES list +
#     the generated trio reference work.config / work.clk_config. The
#     vhdl_list.txt target generates config.vhd from the board's CONFIG_* vars
#     (it does not need the socgen jar — the committed trio is synthesized as-is).
make ulx3s TARGET=vhdl_list.txt

# 3. file list: the generated config + clk_config FIRST, then the ecp5 clkgen
#    arch pad_ring binds (clkgen(ecp5)/EHXPLLL — the real ECP5 PLL primitive
#    yosys supplies via synth_ecp5), then the shared FILES list (which now ends
#    with the generated devices/soc/pad_ring trio — the synthesized board top).
source targets/boards/ulx3s/filelist.sh   # defines FILES=( ... )
FILES=(output/ulx3s/config/config.vhd targets/clk_config.vhd "${FILES[@]}" \
       targets/boards/ulx3s/ulx3s_clkgen_ecp5.vhd)

# Fail fast if the generated SoC does not match the requested VARIANT: VARIANT
# only TAGS the emitted metrics, so a VARIANT that doesn't match the generated
# cpus_config would mislabel them. (CI regenerates per-variant before this runs;
# this guards local runs.) cpus_config.vhd is the soc_gen-generated binding.
VARIANT="${VARIANT:-j2-direct}"
targets/boards/ulx3s/check_variant.sh targets/boards/ulx3s/cpus_config.vhd "$VARIANT"

# 4. synthesize to ECP5 JSON. --syn-binding: component->same-name-entity default
#    binding (clkgen/uart). chformal/delete strip VHDL assert cells nextpnr rejects.
# No --latches: gen_synth_sources.sh rewrites the cache transparent latches as
# negedge FFs (the latch form becomes a LUT-feedback combinational loop that
# nextpnr's timing analyzer rejects). A clean ghdl --synth without --latches
# therefore also proves no stray latch (hence no comb loop) remains.
GHDL_BASE="ghdl --std=93 -fexplicit -fsynopsys --syn-binding --workdir=$WORK ${FILES[*]}"
yosys -m ghdl -p "$GHDL_BASE -e pad_ring; synth_ecp5 -top pad_ring; check -assert; \
  chformal -remove; delete t:\$check t:\$print; stat; write_json $OUT/ulx3s.json" \
  2>&1 | tee "$OUT/yosys.log"

# 5. place & route + pack. --timing-allow-fail keeps producing a bitstream + Fmax
#    even on a timing miss (so metrics still parse); the gate (step 7) fails the
#    build afterwards. nextpnr logs to stderr, so tee 2>&1.
#    The full M2 SoC (CPU+caches+SDRAM+AIC+GPIO) is congestion-limited on the
#    -6 85F: bias placement toward timing (--placer-heap-timingweight, default
#    23) to recover Fmax on the CPU cache-load critical path.
nextpnr-ecp5 --85k --package CABGA381 \
  --json "$OUT/ulx3s.json" --lpf targets/boards/ulx3s/ulx3s.lpf \
  --placer-heap-timingweight 35 \
  --timing-allow-fail --textcfg "$OUT/ulx3s.config" 2>&1 | tee "$OUT/nextpnr.log"
ecppack "$OUT/ulx3s.config" "$OUT/ulx3s.bit"
echo "built $OUT/ulx3s.bit"

# 6. emit synthesis metrics (utilisation + Fmax) for the dashboard.
COMMIT="${GITHUB_SHA:-$(git rev-parse HEAD)}"
python3 tools/fpga/emit_metrics.py --board ulx3s --variant "$VARIANT" --commit "$COMMIT" \
  --nextpnr "$OUT/nextpnr.log" --out "$OUT/metrics.json"

# 7. timing gate: fail the build only if a constrained clock misses its declared
#    frequency at FINAL (post-route) timing. The bitstream + metrics above are
#    already written, so local inspection still works.
#    nextpnr (under --timing-allow-fail) prints the per-clock
#    "Max frequency ... (PASS/FAIL at N MHz)" report MULTIPLE times — intermediate
#    estimates after placement and each route iteration, then the final post-route
#    values. Only the LAST verdict per clock is authoritative (the same "last wins"
#    rule emit_metrics uses for Fmax); grepping for any "(FAIL at ...)" line
#    false-positives on a superseded intermediate estimate (e.g. j4-rom reported an
#    early 19.78 MHz FAIL on sdram_clk but a final 28.77 MHz PASS).
#    A nextpnr crash (not a timing miss) fails earlier via pipefail / ecppack.
if ! awk '
  /Max frequency for clock/ {
    line = $0; sub(/.*clock '\''/, "", line); sub(/'\''.*/, "", line)
    last[line] = ($0 ~ /\(FAIL at/) ? "FAIL" : "PASS"
  }
  END { bad = 0; for (c in last) if (last[c] == "FAIL") { print "  " c > "/dev/stderr"; bad = 1 } exit bad }
' "$OUT/nextpnr.log"; then
  echo "TIMING GATE: a constrained clock misses its declared frequency at final timing (see $OUT/nextpnr.log):" >&2
  exit 1
fi
echo "timing OK (all constrained clocks meet their declared frequency at final timing)"
