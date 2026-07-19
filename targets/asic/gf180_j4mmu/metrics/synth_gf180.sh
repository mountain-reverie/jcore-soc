#!/usr/bin/env bash
# Per-macro GF180 synth-only metrics: ghdl -> yosys generic synth -> map to
# gf180mcu Liberty -> stat. NO place & route. Emits $OUT/<macro>.stat.txt.
#
# MEMORY-AS-FLOPS CAVEAT (read before trusting these numbers): this design's
# cache/register-file RAM (lib/memory_tech_lib/tech/inferred/ram_{1,2}rw_infer.vhd)
# is a classic VHDL indexed-array-in-a-clocked-process inference pattern. The
# ghdl-yosys `-e` elaboration step (GHDL's own internal synthesis, which runs
# BEFORE yosys ever sees the design) expands that pattern directly into
# per-bit flip-flops -- confirmed by `ls` immediately after `-e <top>`: no
# ram_1rw/ram_2rw/ram_*_infer module survives as a separate yosys module, and
# `stat` always reports "Number of memories: 0". There is therefore no yosys
# module boundary left to `blackbox` post-elaboration, and stopping `synth`
# before its `memory_map` step (`-run :coarse`) does not help either -- it
# skips `fine`'s techmap/abc mapping too, so nothing gets mapped to gf180
# cells and $mem never appears anyway (verified: memory bits stayed 0 and the
# flop count was unchanged). This is a GHDL-elaboration-time limitation, not
# a yosys `blackbox`/selection mistake -- fixing it needs GHDL to preserve
# memory arrays as RTLIL $mem (or the RTL to model RAM behind a component
# GHDL's synth treats specially), which is out of scope for this synth-only
# metrics flow. See macros.list for the mitigation adopted instead: every
# stat.txt captures the `stat -liberty` "of which used for sequential
# elements: NN.NN%" line, so a macro whose number is memory-inflated is
# visible at a glance (icache/dcache/cpu are 32-58% sequential -- i.e. RAM-
# as-flops dominated; devices/sdram_ctrl are single-digit -- logic-only, not
# memory-inflated). Plan 2 (real GF180 SRAM macros via place & route) is the
# fix; until then treat icache/dcache/cpu absolute area as an upper bound,
# not the real logic-only footprint.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../../../.." && pwd)"; cd "$ROOT"
OUT="${OUT:-targets/asic/gf180_j4mmu/build}"; mkdir -p "$OUT"
: "${GF180_LIB:?set GF180_LIB to the gf180mcu_fd_sc_mcu7t5v0 typical Liberty}"
source targets/asic/gf180_j4mmu/metrics/gen_synth_sources.sh   # exports GHDL_BASE
# Third (optional) column: the module name `synth -top` should report on,
# when it differs from the ghdl -e elaboration target (e.g. j4_core elaborates
# a per-variant configuration but the underlying module is still `cpu`).
# Defaults to the elaboration target.
while read -r macro elab_top synth_top rest; do
  case "$macro" in ''|\#*) continue;; esac
  synth_top="${synth_top:-$elab_top}"
  yosys -m ghdl -p "$GHDL_BASE -e $elab_top; synth -top $synth_top -flatten; \
    dfflibmap -liberty $GF180_LIB; abc -liberty $GF180_LIB; \
    stat -liberty $GF180_LIB" 2>&1 | tee "$OUT/$macro.yosys.log" \
    | awk '
        /Number of cells/ { buf = ""; capture = 1 }
        capture { buf = buf $0 "\n" }
        /of which used for sequential elements/ { last = buf; capture = 0 }
        END { printf "%s", last }
      ' > "$OUT/$macro.stat.txt"
done < targets/asic/gf180_j4mmu/metrics/macros.list
