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

# 2. generated sources shared with ulx3s: cpu (decode generate + v2p),
# uartlite, cache/bus cores (mirrors targets/boards/ulx3s/sim.sh step 3 and
# the former sim/rtl.sh step 3).
make -C components/cpu/decode generate
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
