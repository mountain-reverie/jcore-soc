#!/usr/bin/env bash
# Analyze + elaborate the iCESugar EBR-only J1 design under ghdl. The hard gate
# for the board VHDL is that `ghdl -e icesugar_top` elaborates with no unbound
# entity (cpus / ice_clkgen / soc). Full nextpnr synthesis is synth.sh.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../../.." && pwd)"
WORK="${WORK:-/tmp/icework}"
cd "$ROOT"
rm -rf "$WORK"; mkdir -p "$WORK"

# 1. generated cpu sources: decode tables (generate) + v2p of the templated
#    cores, plus the v2p'd uart / gpio2 peripherals. Must precede soc_gen so the
#    VHDL library it parses is complete (otherwise it emits a degenerate soc).
make -C components/cpu/decode generate
( cd components/cpu && for f in core/mult core/datapath decode/decode_core; do
    LD_LIBRARY_PATH='' perl ../../tools/v2p < "$f.vhm" > "$f.vhd"; done )
LD_LIBRARY_PATH='' perl tools/v2p < components/uartlite/uart.vhm > components/uartlite/uart.vhd
LD_LIBRARY_PATH='' perl tools/v2p < components/misc/gpio2.vhm > components/misc/gpio2.vhd

# 2. soc_gen: regenerate devices.vhd / soc.vhd / cpus_config.vhd /
#    cpu_synth_files.list / icesugar.pcf from design.yaml.
make icesugar TARGET=soc_gen

# 3. analyze the full design and elaborate the board top.
source targets/boards/icesugar/filelist.sh   # defines FILES=( ... )
ghdl -a --std=93 -fexplicit -fsynopsys --workdir="$WORK" "${FILES[@]}"
ghdl -e --std=93 -fexplicit -fsynopsys --syn-binding --workdir="$WORK" icesugar_top
echo "icesugar_top elaborated OK"
