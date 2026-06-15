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

# 2. cpu sources (decode generate + v2p)
make -C components/cpu/decode generate
( cd components/cpu && for f in core/mult core/datapath decode/decode_core; do
    LD_LIBRARY_PATH='' perl ../../tools/v2p < "$f.vhm" > "$f.vhd"; done )

# 3. full design analyze (real banner boot_image_pkg) + banner testbench
echo "=== ulx3s_top_tb ==="
source targets/boards/ulx3s/filelist.sh   # defines FILES=( ... )
GHDL="ghdl -a --std=93 -fexplicit -fsynopsys --workdir=$WORK"
$GHDL "${FILES[@]}"
$GHDL targets/boards/ulx3s/tb/ulx3s_top_tb.vhd
ghdl -e --std=93 -fexplicit -fsynopsys --syn-binding --workdir="$WORK" ulx3s_top_tb
ghdl -r --std=93 -fexplicit -fsynopsys --syn-binding --workdir="$WORK" ulx3s_top_tb --stop-time=5ms --assert-level=error

# 4. bootram unit testbench (separate work lib: uses the deadbeef test image)
echo "=== bootram_infer_tb ==="
UG="ghdl -a --std=93 -fexplicit -fsynopsys --workdir=$UNITWORK"
$UG components/cpu/cpu2j0_pkg.vhd \
   targets/boards/ulx3s/tb/boot_image_pkg_test.vhd \
   components/memory/bootram_infer.vhd \
   targets/boards/ulx3s/tb/bootram_infer_tb.vhd
ghdl -e --std=93 -fexplicit -fsynopsys --syn-binding --workdir="$UNITWORK" bootram_infer_tb
ghdl -r --std=93 -fexplicit -fsynopsys --syn-binding --workdir="$UNITWORK" bootram_infer_tb --stop-time=20us --assert-level=error
