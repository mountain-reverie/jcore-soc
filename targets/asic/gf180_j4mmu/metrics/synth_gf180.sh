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
# Vendor single-port SRAM Liberty (gf180mcu_fd_ip_sram), derived from the
# standard-cell Liberty's PDK location. Used to give the cache_tag macro's
# vendor black boxes real area (see the cache_tag special case below).
GF180_SRAM_LIB="${GF180_SRAM_LIB:-$(dirname "$GF180_LIB")/../../gf180mcu_fd_ip_sram/lib/gf180mcu_fd_ip_sram__sram256x8m8wm1__tt_025C_5v00.lib}"
# Same vendor SRAM family, 512x8 variant -- used by the cache_data macro's
# 8x sram512x8 tiling (see ram_2x8x2048_2rw_gf180.vhd).
GF180_SRAM_LIB_512="${GF180_SRAM_LIB_512:-$(dirname "$GF180_LIB")/../../gf180mcu_fd_ip_sram/lib/gf180mcu_fd_ip_sram__sram512x8m8wm1__tt_025C_5v00.lib}"

while read -r macro elab_top synth_top rest; do
  case "$macro" in ''|\#*) continue;; esac
  synth_top="${synth_top:-$elab_top}"

  # cache_tag: the cache TAG RAM (ram_2x8x256_1rw) synthesized STANDALONE onto
  # the tech/gf180 vendor hard IP (gf180mcu_fd_ip_sram), NOT through the SoC
  # source set above (which uses tech/inferred). This reports the tag's REAL
  # vendor-macro silicon area (2x sram256x8 + a few glue cells) as its own
  # series -- unlike icache/dcache, whose adapter-level numbers stay inferred/
  # memory-inflated until the data-RAM arbiter lands. read_liberty -lib gives
  # the vendor macro area; stat sums it with the std-cell glue.
  if [ "$macro" = "cache_tag" ]; then
    yosys -m ghdl -p "ghdl --std=93 -fexplicit -fsynopsys --syn-binding \
        --workdir=$OUT/tagwork \
        lib/memory_tech_lib/memory_pkg.vhd \
        lib/memory_tech_lib/ram_2x8x256_1rw.vhd \
        lib/memory_tech_lib/tech/gf180/gf180mcu_fd_ip_sram_comp.vhd \
        lib/memory_tech_lib/tech/gf180/ram_2x8x256_1rw_gf180.vhd \
        -e ram_2x8x256_1rw; read_liberty -lib $GF180_SRAM_LIB; \
      synth -top ram_2x8x256_1rw -flatten; \
      dfflibmap -liberty $GF180_LIB; abc -liberty $GF180_LIB; \
      stat -liberty $GF180_LIB -liberty $GF180_SRAM_LIB" 2>&1 | tee "$OUT/$macro.yosys.log" \
      | awk '
          /Number of cells/ { buf = ""; capture = 1 }
          capture { buf = buf $0 "\n" }
          /of which used for sequential elements/ { last = buf; capture = 0 }
          /Chip area for module/ { last = buf; capture = 0 }
          END { printf "%s", last }
        ' > "$OUT/$macro.stat.txt"
    rm -rf "$OUT/tagwork"
    continue
  fi

  # cache_data: the cache DATA RAM (ram_2x8x2048_2rw) synthesized STANDALONE
  # onto the tech/gf180 vendor hard IP (gf180mcu_fd_ip_sram), analogous to
  # the cache_tag special case above but tiling 8x sram512x8 (4 row-banks x
  # 2 byte-columns) via ram_2x8x2048_2rw_gf180.vhd's mux/row-decode. Reports
  # the DATA RAM's REAL vendor-macro silicon area, not memory-inflated.
  if [ "$macro" = "cache_data" ]; then
    yosys -m ghdl -p "ghdl --std=93 -fexplicit -fsynopsys --syn-binding \
        --workdir=$OUT/datawork \
        lib/memory_tech_lib/memory_pkg.vhd \
        lib/memory_tech_lib/ram_2x8x2048_2rw.vhd \
        lib/memory_tech_lib/tech/gf180/gf180mcu_fd_ip_sram_comp.vhd \
        lib/memory_tech_lib/tech/gf180/ram_2x8x2048_2rw_gf180.vhd \
        -e ram_2x8x2048_2rw; read_liberty -lib $GF180_SRAM_LIB_512; \
      synth -top ram_2x8x2048_2rw -flatten; \
      dfflibmap -liberty $GF180_LIB; abc -liberty $GF180_LIB; \
      stat -liberty $GF180_LIB -liberty $GF180_SRAM_LIB_512" 2>&1 | tee "$OUT/$macro.yosys.log" \
      | awk '
          /Number of cells/ { buf = ""; capture = 1 }
          capture { buf = buf $0 "\n" }
          /of which used for sequential elements/ { last = buf; capture = 0 }
          /Chip area for module/ { last = buf; capture = 0 }
          END { printf "%s", last }
        ' > "$OUT/$macro.stat.txt"
    rm -rf "$OUT/datawork"
    continue
  fi

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
