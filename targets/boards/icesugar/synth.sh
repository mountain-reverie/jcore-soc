#!/usr/bin/env bash
# Build the iCESugar bitstream: ghdl + yosys synth_ice40 -> nextpnr-ice40 (UP5K,
# SG48) -> icepack. The board top is the soc_gen-generated pad_ring; the
# soc_gen-emitted icesugar.pcf names pins (pin_* prefix) that match pad_ring's
# ports directly, so it is passed to nextpnr as-is (no prefix rewrite).
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../../.." && pwd)"
WORK="${WORK:-/tmp/icesynth}"
OUT="${OUT:-$ROOT/targets/boards/icesugar/build}"
cd "$ROOT"; rm -rf "$WORK" "$OUT"; mkdir -p "$WORK" "$OUT"

# 1. generated cpu sources (decode generate + v2p) + v2p'd peripherals. Must
#    precede soc_gen so the VHDL library it parses is complete.
make -C components/cpu/decode generate
( cd components/cpu && for f in core/mult core/datapath decode/decode_core; do
    LD_LIBRARY_PATH='' perl ../../tools/v2p < "$f.vhm" > "$f.vhd"; done )
LD_LIBRARY_PATH='' perl tools/v2p < components/uartlite/uart.vhm > components/uartlite/uart.vhd
LD_LIBRARY_PATH='' perl tools/v2p < components/misc/gpio2.vhm > components/misc/gpio2.vhd

# 2. soc_gen: regenerate the SoC + pcf.
make icesugar TARGET=soc_gen

# 2b. soc_gen direction-inference fixup (Task 8): ice_spi_io's pin_* ports are
# uniformly `inout` (they wrap a shared SB_IO PACKAGE_PIN primitive), so
# soc_gen's bare-signal pin-direction inference can't tell MISO (input-only,
# OUTPUT_ENABLE tied '0' inside ice_spi_io) apart from CS#/SCK/MOSI
# (outputs), and it infers pin_spi_miso_pin as an OUTPUT. Fix the generated
# pad_ring.vhd port direction (and correspondingly flip the assignment
# direction) so nextpnr sees a real input pad instead of an undriven output.
# See task-8-report.md for the full analysis.
sed -i \
  -e 's/pin_spi_miso_pin : out std_logic;/pin_spi_miso_pin : in std_logic;/' \
  -e 's/pin_spi_miso_pin <= spi_miso_pin;/spi_miso_pin <= pin_spi_miso_pin;/' \
  targets/boards/icesugar/pad_ring.vhd

# 3. file list.
source targets/boards/icesugar/filelist.sh   # defines FILES=( ... )

# 4. synthesize to iCE40 JSON, place & route on the UP5K (SG48), pack.
GHDL_BASE="ghdl --std=93 -fexplicit -fsynopsys --syn-binding --workdir=$WORK ${FILES[*]}"
COMMIT="$(git rev-parse --short HEAD 2>/dev/null || echo unknown)"
# synth_ice40 maps the design; then strip the VHDL assert cells ($check/$print/
# $assert) that nextpnr-ice40 rejects before writing the json it consumes.
# stat output is captured in yosys.log so emit_metrics.py can read LC/RAM counts
# even when nextpnr later fails to place.
yosys -m ghdl -p "$GHDL_BASE -e pad_ring; synth_ice40 -top pad_ring -abc2; \
  check -assert; chformal -remove; delete t:\$check t:\$print; stat; \
  write_json $OUT/icesugar.json" 2>&1 | tee "$OUT/yosys.log"

# nextpnr may fail to place (board currently ~103% over): do NOT abort the script
# before metrics are emitted. Capture its log; success also produces the bitstream.
set +e
nextpnr-ice40 --up5k --package sg48 --json "$OUT/icesugar.json" \
  --pcf targets/boards/icesugar/icesugar.pcf --asc "$OUT/icesugar.asc" 2>&1 | tee "$OUT/nextpnr.log"
PNR_RC=${PIPESTATUS[0]}
set -e
[ "$PNR_RC" -eq 0 ] && icepack "$OUT/icesugar.asc" "$OUT/icesugar.bin" && echo "built $OUT/icesugar.bin" || \
  echo "icesugar: nextpnr did not place (rc=$PNR_RC) — emitting yosys-stat metrics only" >&2

python3 tools/fpga/emit_metrics.py --flow ice40 --board icesugar --variant j1 \
  --commit "$COMMIT" --yosys-stat "$OUT/yosys.log" \
  --nextpnr "$OUT/nextpnr.log" \
  --out "$OUT/metrics.json"

# Fit + timing gate. Metrics are already written above, so a failing build still
# uploads them (CI marks this job's metrics upload if:always). The gate fails the
# build on a non-fit (over UP5K budget / nextpnr did not place) or a timing miss.
targets/boards/icesugar/fit_gate.sh "$OUT/nextpnr.log" "$OUT/icesugar.bin"
