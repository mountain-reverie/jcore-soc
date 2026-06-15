#!/usr/bin/env bash
# Build the ULX3S bitstream: ghdl+yosys synth_ecp5 -> nextpnr-ecp5 -> ecppack,
# emit dashboard metrics, and GATE on declared-clock timing closure.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../../.." && pwd)"
WORK="${WORK:-/tmp/m0synth}"
OUT="${OUT:-$ROOT/targets/boards/ulx3s/build}"
cd "$ROOT"; rm -rf "$WORK" "$OUT"; mkdir -p "$WORK" "$OUT"

# 1. boot image
make -C targets/boards/ulx3s/rom
perl tools/genbootpkg targets/boards/ulx3s/rom/main.bin 4096 > targets/boards/ulx3s/boot_image_pkg.vhd

# 2. generated sources: cpu (decode generate + v2p) and uartlite uart.vhd
make -C components/cpu/decode generate
( cd components/cpu && for f in core/mult core/datapath decode/decode_core; do
    LD_LIBRARY_PATH='' perl ../../tools/v2p < "$f.vhm" > "$f.vhd"; done )
LD_LIBRARY_PATH='' perl tools/v2p < components/uartlite/uart.vhm > components/uartlite/uart.vhd

# 3. file list: the sim list plus the ecp5 clkgen arch (analyzed last so default
#    binding selects clkgen(ecp5) over clkgen(sim)).
source targets/boards/ulx3s/filelist.sh   # defines FILES=( ... )
FILES+=(targets/boards/ulx3s/ulx3s_clkgen_ecp5.vhd)

# 4. synthesize to ECP5 JSON. --syn-binding: component->same-name-entity default
#    binding (clkgen/uart). chformal/delete strip VHDL assert cells nextpnr rejects.
GHDL_BASE="ghdl --std=93 -fexplicit -fsynopsys --syn-binding --workdir=$WORK ${FILES[*]}"
yosys -m ghdl -p "$GHDL_BASE -e ulx3s_top; synth_ecp5 -top ulx3s_top; check -assert; \
  chformal -remove; delete t:\$check t:\$print; stat; write_json $OUT/ulx3s.json" \
  2>&1 | tee "$OUT/yosys.log"

# 5. place & route + pack. --timing-allow-fail keeps producing a bitstream + Fmax
#    even on a timing miss (so metrics still parse); the gate (step 7) fails the
#    build afterwards. nextpnr logs to stderr, so tee 2>&1.
nextpnr-ecp5 --85k --package CABGA381 \
  --json "$OUT/ulx3s.json" --lpf targets/boards/ulx3s/ulx3s.lpf \
  --timing-allow-fail --textcfg "$OUT/ulx3s.config" 2>&1 | tee "$OUT/nextpnr.log"
ecppack "$OUT/ulx3s.config" "$OUT/ulx3s.bit"
echo "built $OUT/ulx3s.bit"

# 6. emit synthesis metrics (utilisation + Fmax) for the dashboard.
COMMIT="${GITHUB_SHA:-$(git rev-parse HEAD)}"
python3 tools/fpga/emit_metrics.py --board ulx3s --commit "$COMMIT" \
  --nextpnr "$OUT/nextpnr.log" --out "$OUT/metrics.json"

# 7. timing gate: fail the build if nextpnr reported a timing violation on any
#    constrained clock (declared-clock closure). The bitstream + metrics above are
#    already written, so local inspection still works; CI marks the commit red and
#    the benchmark job (needs: bitstream) skips it (failing commits don't chart).
#    Relies on nextpnr always printing "(PASS at N MHz)"/"(FAIL at N MHz)" under
#    --timing-allow-fail; a nextpnr crash (not a timing miss) fails the pipeline
#    earlier via pipefail (the `| tee` above) / ecppack, so it cannot false-green.
if grep -qE '\(FAIL at [0-9.]+ *MHz\)' "$OUT/nextpnr.log"; then
  echo "TIMING GATE: nextpnr reports a timing violation (see $OUT/nextpnr.log):" >&2
  grep -E '\(FAIL at [0-9.]+ *MHz\)' "$OUT/nextpnr.log" >&2
  exit 1
fi
echo "timing OK (all constrained clocks meet their declared frequency)"
