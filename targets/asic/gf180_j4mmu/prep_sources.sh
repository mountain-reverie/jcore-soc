#!/usr/bin/env bash
# Shared source-generation prep for gf180_j4mmu: regenerate the SoC from
# design.yaml and the generated/v2p'd VHDL sources that both sim/rtl.sh and
# metrics/synth_gf180.sh (via gen_synth_sources.sh -> filelist.sh) need but
# which are NOT committed to the repo (decode tables, v2p outputs). Fixes
# final-review C1: a fresh clone's synth job used to abort deep inside
# gen_synth_sources.sh/filelist.sh on missing generated files because only
# sim/rtl.sh ran this prep. Extracted from sim/rtl.sh steps 0-3 (the
# non-sim-specific, SoC + shared-source generation part; the boot-ROM image
# build stays in rtl.sh -- it's sim-only, not needed for synth).
#
# Usage: targets/asic/gf180_j4mmu/prep_sources.sh
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../../.." && pwd)"
cd "$ROOT"

BD=targets/asic/gf180_j4mmu

# 0. board-discovery guard: targets/boards/gf180_j4mmu must resolve to this
# directory (soc_gen's board loader and the top-level Makefile's board
# discovery both hardcode targets/boards/<name> -- see BD/README.md). Fail
# loudly here rather than deep inside a confusing soc_gen/make error if a
# future board-discovery refactor drops or breaks the symlink.
LINK=targets/boards/gf180_j4mmu
if [ ! -L "$LINK" ]; then
  echo "ERROR: $LINK is missing or not a symlink (expected -> ../asic/gf180_j4mmu)." >&2
  echo "       soc_gen's board loader and the top-level Makefile board discovery" >&2
  echo "       both hardcode targets/boards/<name>; without this shim symlink" >&2
  echo "       'make gf180_j4mmu ...' cannot find this target. See $BD/README.md." >&2
  exit 1
fi
if [ ! -e "$LINK" ]; then
  echo "ERROR: $LINK exists but does not resolve (dangling symlink)." >&2
  exit 1
fi
RESOLVED="$(cd "$LINK" && pwd -P)"
EXPECTED="$(cd "$BD" && pwd -P)"
if [ "$RESOLVED" != "$EXPECTED" ]; then
  echo "ERROR: $LINK resolves to $RESOLVED, expected $EXPECTED." >&2
  exit 1
fi

# 1. regenerate the SoC + clock config from design.yaml.
make gf180_j4mmu TARGET=soc_gen
make gf180_j4mmu TARGET=vhdl_list.txt

# 2. decoder: regenerate THIS BOARD'S VARIANT decoder out-of-tree, via the
# same components/cpu/Makefile.inc mechanism the top-level Makefile's board
# dispatch uses -- NOT `make -C components/cpu/decode generate` (which
# writes the BASE, no-overlay generation into the committed decode/ tree).
# This target is always model:j4 (see design.yaml), and the base decoder
# has no LDTLB/PTEH/PTEL/ASIDR (General Illegal) -- using it here would
# synthesize PRIV_ARCH RTL (TLB, MMU CSRs) with no way to reach it from
# software, the exact bug this out-of-tree per-variant mechanism exists to
# fix. CPU_VARIANT is read the same way the top-level Makefile's board
# dispatch reads it (targets/boards/<board>/build.mk's soc_gen-written
# CPU_VARIANT line, from step 1's `soc_gen` run above; default j2 if
# absent, though this target always has j4). filelist.sh (via
# cpu_synth_files.list) and this generation must agree on CPU_VARIANT, or
# the decoder ends up split across two directories again.
_CPU_VARIANT="$(sed -n 's/^CPU_VARIANT *:= *//p' targets/boards/gf180_j4mmu/build.mk 2>/dev/null)"
_CPU_VARIANT="${_CPU_VARIANT:-j2}"
make -f components/cpu/build.mk VHDLS=CPU_DECODE_BUILD_TMP CPU_VARIANT="$_CPU_VARIANT" \
  "components/cpu/gen/$_CPU_VARIANT/decode/decode_pkg.vhd" \
  "components/cpu/gen/$_CPU_VARIANT/decode/decode.vhd" \
  "components/cpu/gen/$_CPU_VARIANT/decode/decode_body.vhd" \
  "components/cpu/gen/$_CPU_VARIANT/decode/decode_table_simple.vhd" \
  "components/cpu/gen/$_CPU_VARIANT/decode/decode_table_direct.vhd" \
  "components/cpu/gen/$_CPU_VARIANT/decode/decode_table_rom.vhd" \
  "components/cpu/gen/$_CPU_VARIANT/decode/.rom_width_72"

# 2b. generated sources shared with ulx3s: uartlite, cache/bus cores, cpu
# mult/datapath/decode_core v2p (mirrors targets/boards/ulx3s/sim.sh step 3
# and the former sim/rtl.sh step 3). decode_core.vhd is v2p'd here (NOT
# cpugen-generated, unlike the six above -- see decode/Makefile's GENERATED
# list), so it stays in-tree at components/cpu/decode/decode_core.vhd.
( cd components/cpu && for f in core/mult core/datapath decode/decode_core; do
    LD_LIBRARY_PATH='' perl ../../tools/v2p < "$f.vhm" > "$f.vhd"; done )
LD_LIBRARY_PATH='' perl tools/v2p < components/uartlite/uart.vhm > components/uartlite/uart.vhd
for f in components/cpu/cache/dcache_ccl components/cpu/cache/dcache_mcl \
         components/cpu/cache/icache_ccl components/cpu/cache/icache_mcl \
         components/cpu/cache/icache_modereg \
         components/misc/bus_mux_typecsub components/misc/bus_mux_typec \
         components/misc/gpio2 components/misc/spi2; do
  LD_LIBRARY_PATH='' perl tools/v2p < "$f.vhm" > "$f.vhd"
done
LD_LIBRARY_PATH='' perl tools/v2p < targets/cpumreg.vhm > targets/cpumreg.vhd
