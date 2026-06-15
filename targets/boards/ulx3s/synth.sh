#!/usr/bin/env bash
# Build the M0 ULX3S bitstream: ghdl+yosys synth_ecp5 -> nextpnr-ecp5 -> ecppack.
# Timing is report-not-gate for M0 (nextpnr --timing-allow-fail).
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
  chformal -remove; delete t:\$check t:\$print; stat; write_json $OUT/ulx3s.json"

# 5. place & route + pack (CI; absent locally)
nextpnr-ecp5 --85k --package CABGA381 \
  --json "$OUT/ulx3s.json" --lpf targets/boards/ulx3s/ulx3s.lpf \
  --timing-allow-fail --textcfg "$OUT/ulx3s.config"
ecppack "$OUT/ulx3s.config" "$OUT/ulx3s.bit"
echo "built $OUT/ulx3s.bit"
