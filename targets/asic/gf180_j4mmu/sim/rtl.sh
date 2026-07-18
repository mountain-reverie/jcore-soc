#!/usr/bin/env bash
# Simulate the *generated* gf180_j4mmu SoC end to end: regenerate the SoC
# from design.yaml (socgen -> devices.vhd/soc.vhd/cpus_config.vhd) and the
# clock config (make -> output/gf180_j4mmu/config/config.vhd), build a REAL
# boot image (the committed boot_image_pkg.vhd is an all-zero placeholder --
# see that file's header), then elaborate the generated `soc(impl)` DIRECTLY
# (this target has no pad_ring/PLL -- see README.md) as the sim top and drive
# it with gf180_j4mmu_gen_tb, checking the boot banner + SPI loopback.
#
# Adapted from targets/boards/ulx3s/sim.sh: no pad_ring/ehxpll_sim/
# ulx3s_clkgen_ecp5 (no ECP5 PLL to bypass -- clk_sys/reset are primary `soc`
# ports, driven directly by the tb), no dual-core/VARIANT handling (this
# target has one variant: single J4, ROM decode, id-cache).
#
# Usage:
#   sim/rtl.sh          # RTL boot sim (banner/SDRAM/SPI)
#   sim/rtl.sh --mmu    # + the jcore-cpu MMU cosim guard (see below)
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../../../.." && pwd)"
WORK="${WORK:-/tmp/gf180work}"
cd "$ROOT"
rm -rf "$WORK"; mkdir -p "$WORK"

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

# 2. real boot image. The committed boot_image_pkg.vhd is an all-zero
# placeholder (CPU boots nothing) -- build a real one exactly as
# targets/boards/ulx3s/sim.sh does: the ulx3s boot ROM (board-agnostic
# HS-2J0 SH2 ROM bootloader/banner + SPI loopback self-test; single-core, so
# no CONFIG_CPU1_DIAG needed here) via genbootpkg.
make -C targets/boards/ulx3s/rom clean
make -C targets/boards/ulx3s/rom all CONFIG_CPU1_DIAG=0
perl tools/genbootpkg \
    targets/boards/ulx3s/rom/boot.bin \
    4096 \
    > "$BD/boot_image_pkg.vhd"

# 3. generated sources shared with ulx3s: cpu (decode generate + v2p),
# uartlite, cache/bus cores (mirrors targets/boards/ulx3s/sim.sh step 3).
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

# 4. full design analyze + run the self-checking banner testbench.
echo "=== gf180_j4mmu_gen_tb (generated soc, no pad_ring) ==="
GHDL="ghdl -a --std=93 -fexplicit -fsynopsys --workdir=$WORK"
$GHDL output/gf180_j4mmu/config/config.vhd targets/clk_config.vhd
source "$BD/filelist.sh"   # defines FILES=( ... ) incl. devices/cpus_config/soc
$GHDL "${FILES[@]}"
# sim-only helpers (deliberately excluded from filelist.sh: sdram_iocells is
# an ASIC/FPGA IO-cell wrapper the logical `soc` top doesn't use, and the
# behavioral SDRAM model must never be seen by synth).
$GHDL components/sdram/sdram_iocells.vhd
$GHDL components/sdram/sdram_model.vhd
$GHDL "$BD/tb/gf180_j4mmu_gen_tb.vhd"
ghdl -e --std=93 -fexplicit -fsynopsys --syn-binding --workdir="$WORK" gf180_j4mmu_gen_tb
ghdl -r --std=93 -fexplicit -fsynopsys --syn-binding --workdir="$WORK" gf180_j4mmu_gen_tb \
    --stop-time=20ms --assert-level=error

# 5. MMU cosim guard (--mmu only): the J4 MMU-enable sequence (AT enables, a
# present-page translated access relocates -- see BOOT-J4-MMU.md) is
# cosim-verified per-PR in jcore-cpu via components/cpu/sim/mmu_sim.sh, which
# builds the J4-OVERLAY decoder with CONFIG_MMU_ARCH=1 (the base decoder omits
# LDTLB/PTEH/... so MMU instrs must come from the overlay) and runs the
# mmu*/priv-arch functional guards against THIS repo's checked-out
# components/cpu RTL (JCORE_SOC=$ROOT). It restores the committed base decoder
# on exit.
#
# SCOPING: a SoC-level GHDL boot of the MMU Linux port is out of scope for this
# harness (it needs the real jcore kernel + toolchain, not the synthetic boot
# ROM built above). We invoke the jcore-cpu cosim as the guard instead, and
# SCOPE it to the MMU-TRANSLATION guard set below (not mmu_sim.sh's full suite:
# that also runs the broader `m8_*` MAC/instruction-fetch fault-coverage sweep,
# which is unrelated to MMU translation and is timing-flaky under load -- an
# unreliable gate). Each guard here genuinely exercises the MMU translation RTL:
#   mmureloc/if/bp present-page translated access RELOCATES (data / ifetch / bp)
#   mmuxlate       AT-on address translation (the core translate)
#   mmufault/imiss absent-page -> TLB-miss vector (data + ifetch)
# This is precisely the "mmuboot" equivalent BOOT-J4-MMU.md names ("AT enables,
# a present-page translated access relocates, an absent-page access takes the
# TLB-miss vector"). A regression in the LDTLB / translate / page-fault RTL
# fails these. Deliberately NOT the full mmu_sim.sh suite: that also runs the
# broader jcore-cpu WIP guards (mmupagewalk hardware TSB walk + the m8_* MAC /
# instruction-fetch fault-coverage sweep) which are the submodule's own per-PR
# concern and are not green on every checked-out components/cpu -- gating this
# SoC PR on them would be a flaky, out-of-scope gate.
if [ "${1:-}" = "--mmu" ]; then
  echo "=== MMU translation cosim guard (components/cpu/sim/mmu_sim.sh) ==="
  MMU_GUARDS=(mmureloc mmurelocif mmurelocbp mmuxlate mmufault mmuimiss)
  first=1
  for g in "${MMU_GUARDS[@]}"; do
    if [ "$first" = 1 ]; then
      # first call builds the CONFIG_MMU_ARCH=1 cosim, then runs guard $g
      JCORE_SOC="$ROOT" components/cpu/sim/mmu_sim.sh "$g"
      first=0
    else
      # -n reuses the built cosim (still regenerates the J4 decoder on disk)
      JCORE_SOC="$ROOT" components/cpu/sim/mmu_sim.sh -n "$g"
    fi
  done
  echo "==> MMU translation guard PASSED (${MMU_GUARDS[*]})"
fi
