#!/usr/bin/env bash
# xip_sim.sh -- Task 4 of the QSPI XIP sub-project (the functional gate).
#
# Boots the gf180_j4mmu FLASH variant (VARIANT=flash, see design.flash.yaml
# -- Task 2) end to end in GHDL with a behavioral qspi_flash_model preloaded
# with the Task 4 XIP payload (xip_payload/payload.bin), and asserts that the
# CPU actually fetches+executes it from flash@0x14000000 (Task 3's boot ROM
# vector table) by observing the payload's signature store
# (0x0000005A @ boot-RAM byte 0x100) on the boot-RAM write bus -- see
# tb/cpus_xip_probe.vhd and tb/xip_cosim_tb.vhd for the full mechanism.
#
# IMPORTANT (do not "fix" this away): regenerating the SoC with
# VARIANT=flash overwrites the COMMITTED devices.vhd/soc.vhd/cpus_config.vhd
# /pad_ring.vhd/build.mk with flash-variant content (as designed -- Task 2).
# Those files must stay flash-LESS in git (the base gf180_j4mmu build is
# unaffected -- see sim/rtl.sh's own regen). This script saves them before
# regenerating and restores them (via a trap, so it happens on any exit
# path including failure) when done. It also NEVER touches boot_image_pkg
# .vhd (Task 3's PC=0x14000000/SP=0x3ffc vector table) -- unlike rtl.sh,
# which overwrites it with a synthetic bootloader image, this script does
# not call genbootpkg at all.
#
# Usage:
#   sim/xip_sim.sh
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../../../.." && pwd)"
WORK="${WORK:-/tmp/gf180xipwork}"
cd "$ROOT"
rm -rf "$WORK"; mkdir -p "$WORK"

BD=targets/asic/gf180_j4mmu

# 0. board-discovery guard (same check as prep_sources.sh's step 0).
LINK=targets/boards/gf180_j4mmu
if [ ! -L "$LINK" ] || [ "$(cd "$LINK" && pwd -P)" != "$(cd "$BD" && pwd -P)" ]; then
  echo "ERROR: $LINK is missing or does not resolve to $BD -- see $BD/README.md." >&2
  exit 1
fi

# 1. save the flash-less committed generated files (+ the Task 3 boot ROM,
# which we must not touch at all) and restore them unconditionally on exit.
SAVE="$(mktemp -d)"
cp "$BD/devices.vhd" "$BD/soc.vhd" "$BD/cpus_config.vhd" "$BD/pad_ring.vhd" \
   "$BD/build.mk" "$BD/boot_image_pkg.vhd" "$SAVE/"
restore_generated() {
  cp "$SAVE/devices.vhd" "$SAVE/soc.vhd" "$SAVE/cpus_config.vhd" \
     "$SAVE/pad_ring.vhd" "$SAVE/build.mk" "$SAVE/boot_image_pkg.vhd" "$BD/"
  rm -rf "$SAVE"
}
trap restore_generated EXIT

# 2. shared source generation: reuse prep_sources.sh (shared with
# sim/rtl.sh and metrics/synth_gf180.sh -- see that script's header) for
# the VARIANT-INDEPENDENT generated sources (cpu decode/v2p, uartlite,
# cache/bus cores). prep_sources.sh's own soc_gen call regenerates the
# BASE (flash-less) variant as a side effect -- immediately re-run soc_gen
# with VARIANT=flash afterward to overwrite devices.vhd/soc.vhd/
# cpus_config.vhd/pad_ring.vhd/build.mk with the flash-variant content
# (Task 2's attach) that this cosim actually needs.
"$BD/prep_sources.sh"
make gf180_j4mmu TARGET=soc_gen VARIANT=flash
make gf180_j4mmu TARGET=vhdl_list.txt VARIANT=flash

# 2b. soc_gen's own choice of instantiation form for `cpus` in soc.vhd was
# found EMPIRICALLY NONDETERMINISTIC across otherwise-identical
# `make ... TARGET=soc_gen VARIANT=flash` runs: sometimes it emits the
# unambiguous `cpus : configuration work.soc_cpus_config` form (which
# explicitly names the configuration that threads decode_core's required
# decode_type/reset_vector generics down through cpu_synth_j4 ->
# cpu_decode_direct), sometimes the bare `cpus : entity work.cpus` form
# (no configuration named). `ghdl -e --syn-binding`'s default-binding
# SEARCH for that bare form was, in turn, found EMPIRICALLY UNRELIABLE --
# repeated elaboration of the SAME analyzed library gave inconsistent
# pass/fail resolving those generics ("no actual for generic
# decode_type/reset_vector" on decode.vhd's unrelated `core: decode_core`
# component instantiation). Normalize unconditionally to the
# configuration form -- a plain, safe text substitution (both forms take
# an identical port map; only the "entity work.cpus" / "configuration
# work.soc_cpus_config" head differs) -- so elaboration is deterministic
# regardless of which form THIS run's soc_gen happened to pick.
sed -i 's/cpus : entity work\.cpus/cpus : configuration work.soc_cpus_config/' "$BD/soc.vhd"
if ! command grep -q 'cpus : configuration work\.soc_cpus_config' "$BD/soc.vhd"; then
  echo "ERROR: could not normalize $BD/soc.vhd's cpus instance to" \
       "'configuration work.soc_cpus_config' (soc_gen's cpus instantiation text changed" \
       "shape? update this sed to match)." >&2
  exit 1
fi

# 3. analyze: shared filelist (all of Tasks 1-3's RTL, in devices/soc's
# generated flash-variant form) + the sim-only SDRAM model (still a live
# mem-bus target behind mem_region_mux even in the flash variant) + the
# behavioral flash model + Task 4's cpus probe architecture + the cosim tb.
#
# tb/cpus_xip_probe.vhd REPLACES targets/boards/ulx3s/cpus_one_m0_arch.vhd
# in the analyze list (same architecture name "one_cpu_m0", + the XIP
# signature monitor) -- see that file's header for why: a differently-
# named architecture broke gf180_j4mmu/cpus_config.vhd's `soc_cpus_config`
# configuration (its `for one_cpu_m0` clause, which is what actually
# threads decode_core's required generics down through cpu_synth_j4 ->
# cpu_decode_direct, no longer matched). Filter FILES to drop the ulx3s
# copy so only ONE "one_cpu_m0" architecture exists in the library.
echo "=== xip_cosim_tb (gf180_j4mmu FLASH variant, XIP payload) ==="
GHDL="ghdl -a --std=93 -fexplicit -fsynopsys --workdir=$WORK"
$GHDL output/gf180_j4mmu/config/config.vhd targets/clk_config.vhd
source "$BD/filelist.sh"
# tb/cpus_xip_probe.vhd must be analyzed BEFORE cpus_config.vhd (whose
# `soc_cpus_config` configuration's `for one_cpu_m0` clause needs the
# "one_cpu_m0" architecture to already exist) and, obviously, before
# soc.vhd. Split FILES at devices.vhd (the first of the generated
# devices/cpus_config/soc trio) and drop the ulx3s copy of "one_cpu_m0"
# entirely (see cpus_xip_probe.vhd's header for why a same-named
# replacement, not an additional architecture, is required).
XIP_PRE=()
XIP_POST=()
in_post=0
for f in "${FILES[@]}"; do
  case "$f" in
    targets/boards/ulx3s/cpus_one_m0_arch.vhd) continue ;;
    targets/asic/gf180_j4mmu/devices.vhd) in_post=1 ;;
  esac
  if [ "$in_post" = 1 ]; then XIP_POST+=("$f"); else XIP_PRE+=("$f"); fi
done
$GHDL "${XIP_PRE[@]}"
$GHDL components/sdram/sdram_iocells.vhd
$GHDL components/sdram/sdram_model.vhd
$GHDL components/misc/tests/qspi_flash_model.vhd
$GHDL "$BD/tb/cpus_xip_probe.vhd"
$GHDL "${XIP_POST[@]}"
$GHDL "$BD/tb/xip_cosim_tb.vhd"
ghdl -e --std=93 -fexplicit -fsynopsys --syn-binding --workdir="$WORK" xip_cosim_tb

# 4. run, capture output, and gate PASS on the exact XIP_SIG_OK report (the
# in-architecture assertion from tb/cpus_xip_probe.vhd's xip_monitor
# process -- see that file and xip_cosim_tb.vhd's headers for why there is
# no in-sim severity-failure watchdog: nothing at the tb's own hierarchy
# level can observe the boot-RAM write bus without an external
# name/extra-port hack this design deliberately avoids).
OUT="$WORK/run.log"
ghdl -r --std=93 -fexplicit -fsynopsys --syn-binding --workdir="$WORK" xip_cosim_tb \
    --stop-time=2ms --assert-level=error 2>&1 | tee "$OUT"

if grep -q "XIP_SIG_OK" "$OUT"; then
  echo "==> XIP cosim PASSED: signature 0x0000005A observed at boot-RAM 0x100 -- payload fetched+executed from flash@0x14000000"
else
  echo "==> XIP cosim FAILED: XIP_SIG_OK never observed (see $OUT)" >&2
  exit 1
fi
