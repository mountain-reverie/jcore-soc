#!/usr/bin/env bash
# Build the iCESugar bitstream: ghdl + yosys synth_ice40 -> nextpnr-ice40 (UP5K,
# SG48) -> icepack. The board top is the hand-written icesugar_top; the
# soc_gen-emitted icesugar.pcf names pins for the generated pad_ring (pin_*
# prefix), so we derive an icesugar_top-matching pcf (unprefixed) at build time.
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

# 3. file list.
source targets/boards/icesugar/filelist.sh   # defines FILES=( ... )

# 4. derive an icesugar_top-matching pcf (strip the pad_ring pin_ prefix).
sed 's/set_io pin_/set_io /' targets/boards/icesugar/icesugar.pcf > "$OUT/icesugar.pcf"

# 5. synthesize to iCE40 JSON, place & route on the UP5K (SG48), pack.
GHDL_BASE="ghdl --std=93 -fexplicit -fsynopsys --syn-binding --workdir=$WORK ${FILES[*]}"
# synth_ice40 maps the design; then strip the VHDL assert cells ($check/$print/
# $assert) that nextpnr-ice40 rejects before writing the json it consumes.
yosys -m ghdl -p "$GHDL_BASE -e icesugar_top; synth_ice40 -top icesugar_top; \
  check -assert; chformal -remove; delete t:\$check t:\$print; stat; \
  write_json $OUT/icesugar.json" \
  2>&1 | tee "$OUT/yosys.log"
nextpnr-ice40 --up5k --package sg48 --json "$OUT/icesugar.json" \
  --pcf "$OUT/icesugar.pcf" --asc "$OUT/icesugar.asc" 2>&1 | tee "$OUT/nextpnr.log"
icepack "$OUT/icesugar.asc" "$OUT/icesugar.bin"
echo "built $OUT/icesugar.bin"
