#!/usr/bin/env bash
# Build the EBR boot image, analyze the iCESugar EBR-only J1 design under ghdl,
# and run the top-level banner testbench (drive 12 MHz, decode ser_tx, assert
# the boot banner). Full nextpnr synthesis is synth.sh.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../../.." && pwd)"
WORK="${WORK:-/tmp/icework}"
cd "$ROOT"
rm -rf "$WORK"; mkdir -p "$WORK"

# 0. boot image: cross-compile the standalone banner/blink program (its own
#    crt0 + linker script, no boot/ submodule), then pack the SREC/bin into the
#    bootram_infer init package (c_addr_width = 11 -> 512 words). This
#    overwrites the boot_image_pkg.vhd placeholder with the real program.
make -C targets/boards/icesugar/rom all
perl tools/genbootpkg \
    targets/boards/icesugar/rom/boot.bin \
    512 \
    > targets/boards/icesugar/boot_image_pkg.vhd

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
# SB_MAC16 behavioral model: sim-only stand-in for the iCE40 DSP that
# mult(ice40dsp) instantiates. Synthesis (synth.sh) leaves SB_MAC16 an unbound
# component for yosys to map to the real cell, so this is added ONLY here.
# -C (--mb-comments) allows the multi-byte Unicode arrows/quotes used in
# sb_mac16_sim.vhd's comments; current GHDL (6.0.0) rejects them under
# --std=93 without it, on every ghdl invocation (analyze/elaborate/run) that
# touches this file.
FILES=( components/cpu/core/sb_mac16_sim.vhd components/memory/sb_spram256ka_sim.vhd "${FILES[@]}" )
ghdl -a --std=93 -fexplicit -fsynopsys -C --workdir="$WORK" "${FILES[@]}"
ghdl -e --std=93 -fexplicit -fsynopsys -C --syn-binding --workdir="$WORK" pad_ring
echo "pad_ring elaborated OK"

# 4. top-level banner testbench: drive 12 MHz, decode ser_tx, assert the banner.
echo "=== icesugar_top_tb ==="
ghdl -a --std=93 -fexplicit -fsynopsys -C --workdir="$WORK" \
    targets/boards/icesugar/tb/icesugar_top_tb.vhd
ghdl -e --std=93 -fexplicit -fsynopsys -C --syn-binding --workdir="$WORK" icesugar_top_tb
ghdl -r --std=93 -fexplicit -fsynopsys -C --syn-binding --workdir="$WORK" icesugar_top_tb \
    --stop-time=150ms --assert-level=error
