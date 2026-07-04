#!/usr/bin/env bash
# Two cross-connected iCESugar J1 SoCs (icesugar_pair_tb): build the
# ETH_PAIR_TEST boot image, swap it in for the default boot_image_pkg.vhd,
# and run the pair testbench -- validates real PHY-to-PHY RX end-to-end
# (no SPRAM memtest, no hand-coded Manchester stimulus) in a few ms of sim
# time. Mirrors sim.sh; see that script for step-by-step commentary.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../../.." && pwd)"
WORK="${WORK:-/tmp/iceworkpair}"
cd "$ROOT"
rm -rf "$WORK"; mkdir -p "$WORK"

# 0. boot image: the ETH_PAIR_TEST variant (skips the 128 KB SPRAM memtest,
#    sends a gratuitous ARP request at boot). genbootpkg emits a
#    boot_image_pkg.vhd (same package name as the default image) at a
#    separate path so it doesn't collide with the default one on disk;
#    filelist below is edited to reference this pairtest file instead of
#    the default so only ONE boot_image_pkg gets analyzed into $WORK.
make -C targets/boards/icesugar/rom pairtest
perl tools/genbootpkg \
    targets/boards/icesugar/rom/boot_pairtest.bin \
    512 \
    > targets/boards/icesugar/boot_image_pairtest_pkg.vhd

# 1. generated cpu sources (same as sim.sh).
make -C components/cpu/decode generate
( cd components/cpu && for f in core/mult core/datapath decode/decode_core; do
    LD_LIBRARY_PATH='' perl ../../tools/v2p < "$f.vhm" > "$f.vhd"; done )
LD_LIBRARY_PATH='' perl tools/v2p < components/uartlite/uart.vhm > components/uartlite/uart.vhd
LD_LIBRARY_PATH='' perl tools/v2p < components/misc/gpio2.vhm > components/misc/gpio2.vhd

# 2. soc_gen (same as sim.sh).
make icesugar TARGET=soc_gen

# 3. analyze the full design + pair tb. Same FILES as sim.sh's filelist.sh,
#    but with boot_image_pairtest_pkg.vhd substituted for boot_image_pkg.vhd
#    (both define package boot_image_pkg -- only the pairtest one may be
#    analyzed into this workdir) and icesugar_pair_tb as the sim top.
source targets/boards/icesugar/filelist.sh   # defines FILES=( ... )
BRD=targets/boards/icesugar
for i in "${!FILES[@]}"; do
  if [ "${FILES[$i]}" = "$BRD/boot_image_pkg.vhd" ]; then
    FILES[$i]="$BRD/boot_image_pairtest_pkg.vhd"
  fi
done
FILES=( components/cpu/core/sb_mac16_sim.vhd components/memory/sb_spram256ka_sim.vhd components/emac/sb_pll40_2_pad_sim.vhd "${FILES[@]}" )
ghdl -a --std=93 -fexplicit -fsynopsys -C --workdir="$WORK" "${FILES[@]}"
ghdl -e --std=93 -fexplicit -fsynopsys -C --syn-binding --workdir="$WORK" pad_ring
echo "pad_ring elaborated OK"

echo "=== icesugar_pair_tb ==="
ghdl -a --std=93 -fexplicit -fsynopsys -C --workdir="$WORK" \
    targets/boards/icesugar/tb/icesugar_pair_tb.vhd
ghdl -e --std=93 -fexplicit -fsynopsys -C --syn-binding --workdir="$WORK" icesugar_pair_tb
ghdl -r --std=93 -fexplicit -fsynopsys -C --syn-binding --workdir="$WORK" icesugar_pair_tb \
    --stop-time=16ms --assert-level=error
