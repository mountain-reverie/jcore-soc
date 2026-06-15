#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../../.." && pwd)"
WORK="${WORK:-/tmp/sdramwork}"
cd "$ROOT"; rm -rf "$WORK"; mkdir -p "$WORK"
G="ghdl -a --std=93 -fexplicit -fsynopsys --workdir=$WORK"
$G components/cpu/cpu2j0_pkg.vhd
$G components/sdram/sdram_pkg.vhd
$G components/sdram/sdram_iocells.vhd
$G components/sdram/sdram_model.vhd
$G components/sdram/sdram_ctrl.vhd
$G components/sdram/tests/sdram_model_tb.vhd
$G components/sdram/tests/sdram_ctrl_tb.vhd
for tb in sdram_model_tb sdram_ctrl_tb; do
  ghdl -e --std=93 -fexplicit -fsynopsys --workdir="$WORK" "$tb"
  ghdl -r --std=93 -fexplicit -fsynopsys --workdir="$WORK" "$tb" --stop-time=2ms --assert-level=error
done
echo "SDRAM tests PASSED"
