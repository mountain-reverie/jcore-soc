#!/usr/bin/env bash
# xip_sim.sh -- Task 4 of the QSPI XIP sub-project (the functional gate).
#
# Boots the gf180_j4mmu FLASH variant (VARIANT=flash, see design.flash.yaml
# -- Task 2) end to end in GHDL with a behavioral qspi_flash_model preloaded
# with the Task 4 XIP payload (xip_payload/payload.bin), and asserts that the
# CPU actually fetches+executes it from flash@0x14000000 (Task 3's boot ROM
# vector table) by observing the payload's signature store
# (0xF1A5B007 @ SDRAM byte 0x10000ffc, a push onto the SDRAM stack whose
# pointer comes straight from the reset vector -- boot_mem is now pure
# ROM, no on-chip scratchpad) on the CPU DEV_DDR data bus -- see
# tb/cpus_xip_probe.vhd and tb/xip_cosim_tb.vhd for the full mechanism.
#
# IMPORTANT (do not "fix" this away): regenerating the SoC with
# VARIANT=flash overwrites the COMMITTED devices.vhd/soc.vhd/cpus_config.vhd
# /pad_ring.vhd/build.mk/board.dts with flash-variant content (as designed --
# Task 2; board.dts's "compatible" string flips jcore,j2 -> jcore,j4 under
# VARIANT=flash regen). Those files must stay flash-LESS in git (the base
# gf180_j4mmu build is unaffected -- see sim/rtl.sh's own regen). This script
# saves them before regenerating and restores them (via a trap, so it
# happens on any exit path including failure) when done. It also NEVER
# touches boot_image_pkg.vhd (Task 3's PC=0x14000000/SP=0x00000ffc vector table)
# -- unlike rtl.sh, which overwrites it with a synthetic bootloader image,
# this script does not call genbootpkg at all.
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
   "$BD/build.mk" "$BD/boot_image_pkg.vhd" "$BD/board.dts" "$SAVE/"
restore_generated() {
  cp "$SAVE/devices.vhd" "$SAVE/soc.vhd" "$SAVE/cpus_config.vhd" \
     "$SAVE/pad_ring.vhd" "$SAVE/build.mk" "$SAVE/boot_image_pkg.vhd" \
     "$SAVE/board.dts" "$BD/"
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

# 2c. Task 6 PHASE A: normalize soc.vhd's ddr_ram_mux instantiation to the
# gf180 vendor-SRAM configuration (targets/asic/gf180_j4mmu/
# ddr_ram_mux_one_cpu_idcache_gf180.vhd), same rationale/mechanism as 2b
# above -- socgen's cpumap.go only knows the "_fpga" configuration names
# (tools/socgen/elaborate/cpumap.go), so it always emits either the
# unambiguous ddr_ram_mux_one_cpu_idcache_fpga form or (observed for this
# VARIANT=flash regen) the bare `entity work.ddr_ram_mux` fallback; neither
# names our gf180 configuration, so force it here. Handles both observed
# forms.
# CACHE_MEM=infer (Task 5 gate): bind the _fpga (inferred cache RAM) ddr_ram_mux
# configuration instead of the GF180-vendor-SRAM one -- see the CACHE_MEM
# handling below in the analyze-list splice for why (the _gf180 configuration
# selects icache_adapter_gf180/dcache_adapter_gf180, which this mode does not
# analyze).
DDR_RAM_MUX_CFG="ddr_ram_mux_one_cpu_idcache_gf180"
if [ "${CACHE_MEM:-}" = "infer" ]; then
  DDR_RAM_MUX_CFG="ddr_ram_mux_one_cpu_idcache_fpga"
fi
sed -i \
  -e "s/ddr_ram_mux : entity work\.ddr_ram_mux/ddr_ram_mux : configuration work.$DDR_RAM_MUX_CFG/" \
  -e "s/ddr_ram_mux : configuration work\.ddr_ram_mux_one_cpu_idcache_fpga/ddr_ram_mux : configuration work.$DDR_RAM_MUX_CFG/" \
  -e "s/ddr_ram_mux : configuration work\.ddr_ram_mux_one_cpu_idcache_gf180/ddr_ram_mux : configuration work.$DDR_RAM_MUX_CFG/" \
  "$BD/soc.vhd"
if ! command grep -q "ddr_ram_mux : configuration work\.$DDR_RAM_MUX_CFG" "$BD/soc.vhd"; then
  echo "ERROR: could not normalize $BD/soc.vhd's ddr_ram_mux instance to" \
       "'configuration work.$DDR_RAM_MUX_CFG' (soc_gen's ddr_ram_mux" \
       "instantiation text changed shape? update this sed to match)." >&2
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

# Task 4/5 (2 KB cache_pack spike): CACHE=2k substitutes the target-local
# cache_pkg_2k.vhd (CACHE_INDEX_BITS=6, 2 KB) for the submodule's
# components/cpu/cache/cache_pkg.vhd (CACHE_INDEX_BITS=8, 8 KB) in the
# analyze list -- same package name `cache_pack`, so `work` sees whichever
# one was analyzed. Default (CACHE unset) keeps the committed 8 KB behavior
# byte-identical. Tasks 2-3 added the 64/512-deep inferred RAM primitives
# (memory_layout-selected by ADDR_WIDTH) this 2 KB size needs.
if [ "${CACHE:-}" = "2k" ]; then
  CACHE_FILES=()
  for f in "${FILES[@]}"; do
    case "$f" in
      components/cpu/cache/cache_pkg.vhd)
        CACHE_FILES+=("$BD/cache_pkg_2k.vhd") ;;
      *) CACHE_FILES+=("$f") ;;
    esac
  done
  FILES=("${CACHE_FILES[@]}")
fi
# Task 6 PHASE A/B: splice in the gf180 vendor-SRAM cache RAM chain (mirrors
# metrics/gen_synth_sources.sh's GHDL_BASE_GF180_CACHE construction exactly
# -- drop the tech/inferred ram_{1,2}rw_infer.vhd entries, insert the
# tech/gf180 macro chain + cache_gf180_config.vhd in place of
# cache_config_fpga.vhd), PLUS (new for this cosim, unlike the synth-only
# GHDL_BASE_GF180_CACHE) the SIM-ONLY behavioral stub
# (components/memory/tests/gf180_sram_sim_stub.vhd) so GHDL has a real
# architecture to bind the vendor macro component instances to (it declares
# entities gf180mcu_fd_ip_sram__sram{256,512}x8m8wm1 with architecture
# `sim`, matching gf180mcu_fd_ip_sram_comp.vhd's component declarations by
# name -- GHDL's default component-instantiation binding then resolves to
# it automatically). The stub is NEVER added to filelist.sh itself, so it
# never reaches the LibreLane/synth flow (which leaves the vendor macros
# genuinely unbound -> yosys sees them as black boxes, matched against the
# real macro LEF/lib at P&R time instead).
GF180_MEM_EXTRA=(
  lib/memory_tech_lib/ram_18x2048_1rw.vhd
  lib/memory_tech_lib/ram_32x1x512_2rw.vhd
  lib/memory_tech_lib/ram_2x8x256_1rw.vhd
  lib/memory_tech_lib/ram_2x8x2048_2rw.vhd
  lib/memory_tech_lib/tech/sim/ram_18x2048_1rw_sim.vhd
  lib/memory_tech_lib/tech/sim/ram_32x1x512_2rw_sim.vhd
  lib/memory_tech_lib/tech/gf180/gf180mcu_fd_ip_sram_comp.vhd
  components/memory/tests/gf180_sram_sim_stub.vhd
  lib/memory_tech_lib/tech/gf180/ram_2x8x256_1rw_gf180.vhd
  lib/memory_tech_lib/tech/gf180/ram_2x8x2048_2rw_gf180.vhd
  lib/memory_tech_lib/ram_1rw_mems.vhd
  lib/memory_tech_lib/ram_2rw_mems.vhd
  lib/memory_tech_lib/tech/gf180/mem_gf180_config.vhd
)
# cache_gf180_config.vhd (the icache_adapter_gf180/dcache_adapter_gf180
# configurations) needs icache_ram/dcache_ram/icache_adapter/dcache_adapter
# already analyzed -- those come AFTER the ram_2rw_infer.vhd splice point
# above, so this one is spliced in separately, right after
# cache_config_fpga.vhd (kept -- one_cpu_idcache_fpga.vhd still references
# its icache_adapter_fpga/dcache_adapter_fpga configurations structurally,
# even though the flash variant's soc.vhd is normalized to bind the gf180
# configuration instead; both configurations coexisting is harmless, same
# principle as the rest of this splice).
# Task 5 gate: CACHE_MEM=infer keeps the base FILES list's inferred
# ram_1rw_infer/ram_2rw_infer CACHE RAM archs (tech/inferred,
# memory_layout-selected by ADDR_WIDTH -- Tasks 2-3's 64/512-deep
# primitives for the 2 KB size) instead of substituting the GF180 vendor
# cache macros -- the cheap oracle that runs long before the GF180 64/512
# vendor wrappers exist (a later task). This ONLY affects the cache RAM
# splice points; boot_mem_stack_gf180.vhd (a plain ROM, no vendor macro --
# see its header) is still spliced in unconditionally, same as always,
# since it is unrelated to the cache. Default (CACHE_MEM unset) is
# unchanged: GF180 vendor cache SRAM substitution, as before.
GF180_FILES=()
for f in "${FILES[@]}"; do
  case "$f" in
    lib/memory_tech_lib/tech/inferred/ram_1rw_infer.vhd)
      if [ "${CACHE_MEM:-}" = "infer" ]; then GF180_FILES+=("$f"); fi ;;
    lib/memory_tech_lib/tech/inferred/ram_2rw_infer.vhd)
      if [ "${CACHE_MEM:-}" = "infer" ]; then
        GF180_FILES+=("$f")
      else
        GF180_FILES+=("${GF180_MEM_EXTRA[@]}")
      fi ;;
    components/cpu/cache/cache_config_fpga.vhd)
      GF180_FILES+=("$f")
      if [ "${CACHE_MEM:-}" != "infer" ]; then
        GF180_FILES+=(lib/memory_tech_lib/tech/gf180/cache_gf180_config.vhd)
      fi ;;
    targets/ddr_ram_mux/one_cpu_idcache_fpga.vhd)
      GF180_FILES+=("$f")
      if [ "${CACHE_MEM:-}" != "infer" ]; then
        GF180_FILES+=(targets/asic/gf180_j4mmu/ddr_ram_mux_one_cpu_idcache_gf180.vhd)
      fi ;;
    targets/asic/gf180_j4mmu/cpus_one_m0_gf180_arch.vhd)
      GF180_FILES+=(lib/memory_tech_lib/tech/gf180/boot_mem_stack_gf180.vhd "$f") ;;
    *) GF180_FILES+=("$f") ;;
  esac
done
FILES=("${GF180_FILES[@]}")
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
  echo "==> XIP cosim PASSED: signature 0xF1A5B007 observed at SDRAM 0x10000ffc (SDRAM stack push, no on-chip scratchpad) -- payload fetched+executed from flash@0x14000000"
else
  echo "==> XIP cosim FAILED: XIP_SIG_OK never observed (see $OUT)" >&2
  exit 1
fi
