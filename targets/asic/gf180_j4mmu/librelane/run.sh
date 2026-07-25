#!/usr/bin/env bash
# GF180 LibreLane harness: harden ONE macro (block) through place & route on
# the gf180mcuC PDK. Generalizes components/cpu/synth/openlane/run.sh (same
# docker-or-native invocation, watchdog, OL_TO stop-point) for this
# VHDL-sourced target.
#
# THE VHDL->LIBRELANE PATH (Task 2's key unknown): LibreLane/OpenROAD only
# consume Verilog. We reuse targets/asic/gf180_j4mmu/metrics/gen_synth_sources.sh
# (the synth-clean VHDL copies + soc_port_* attribute stripping already
# proven by the synth-only metrics flow) and run yosys -m ghdl ourselves to
# `synth -flatten` + `write_verilog` a real gate-level netlist BEFORE handing
# off to LibreLane -- i.e. option (a)/(c) hybrid: yosys(ghdl plugin) does
# elaboration+generic synth, LibreLane only does floorplan->place->CTS->route
# (LibreLane's own `Yosys.Synthesis` step is skipped by pointing VERILOG_FILES
# at the pre-synthesized netlist and setting SYNTH_ELABORATE_ONLY-style
# passthrough is NOT needed -- LibreLane's synth step re-synthesizes generic
# Verilog fine, so we hand it a generic-cell (not yet gf180-mapped) netlist
# and let LibreLane's own Yosys.Synthesis map it to gf180 Liberty; this keeps
# LibreLane's own SDC/timing/floorplan steps in charge of the PDK mapping and
# avoids double-mapping). Rationale: ghdl's `--synth` elaboration is the ONLY
# tool in this chain that understands VHDL; everything downstream (LibreLane,
# OpenROAD, Yosys proper) is Verilog-only, so the ghdl-yosys boundary is the
# necessary and sufficient VHDL->Verilog crossing point.
#
# GOTCHA (fixed here): ghdl-yosys's `write_verilog` renders VHDL record
# fields as escaped Verilog identifiers with literal `[` `]` in the NAME
# (e.g. `\req[a] `), not real bit-selects. Verilog/LEF/DEF pin-naming code
# (and OpenROAD's IO placer) can misparse a `[...]` suffix on a port as a bus
# index. We sanitize port names post-write (`req[a]` -> `req_a`) before
# LibreLane ever sees the netlist; internal escaped net names (e.g.
# `\:377.A`) are untouched (they are not pins).
#
# Usage: run.sh macro=<name>
#   <name> selects librelane/<name>/config.json (merged over common.json) and
#   the macro's top-level Verilog netlist is generated into that directory.
# Env:
#   OL_IMAGE   docker image (default ghcr.io/librelane/librelane:2.4.2, the
#              newest tagged stable release confirmed pullable on this box).
#   PDK_ROOT   ciel PDK checkout root (default ~/.ciel; must contain
#              $PDK_ROOT/gf180mcuC per common.json's "PDK").
#   OL_TO      LibreLane step to stop at (default: run to completion / full
#              routing+signoff -- override to stop earlier, e.g.
#              OpenROAD.DetailedPlacement, for a quicker area-only check).
#   OL_TIMEOUT wall-clock cap in seconds (default 3600).
set -uo pipefail
ROOT="$(cd "$(dirname "$0")/../../../.." && pwd)"
HERE="$(cd "$(dirname "$0")" && pwd)"
cd "$ROOT"

MACRO=""
for a in "$@"; do
  case "$a" in
    macro=*) MACRO="${a#macro=}" ;;
  esac
done
if [ -z "$MACRO" ]; then
  echo "usage: $0 macro=<name>  (name -> librelane/<name>/config.json)" >&2
  exit 1
fi

MDIR="$HERE/$MACRO"
MCFG="$MDIR/config.json"
if [ ! -f "$MCFG" ]; then
  echo "ERROR: $MCFG not found" >&2
  exit 1
fi

OL_IMAGE="${OL_IMAGE:-ghcr.io/librelane/librelane:2.4.2}"
PDK_ROOT="${PDK_ROOT:-$HOME/.ciel}"
OL_TIMEOUT="${OL_TIMEOUT:-3600}"
# Default stop point: Magic.WriteLEF (right after detailed routing + LEF
# abstract emission), BEFORE the Checker.{Setup,Hold,MaxCap,MaxSlew}Violations
# steps later in the Classic flow. Those 4 checkers have no ERROR_ON_* opt-out
# (librelane/steps/checker.py TimingViolations subclasses: deferred=True, no
# error_on_var) -- at gf180's slow (180nm) corner + the CPU's deep
# combinational paths, they WILL find setup violations even at a relaxed
# clock, and LibreLane treats that as a fatal deferred error. Since this
# sub-project only needs ROUTED AREA (timing is explicitly non-gating here),
# stopping before those checkers is the only way to get a routed layout +
# LEF/GDS/area out without a real timing-closure effort. Override
# (OL_TO=<step> or OL_TO=  for full signoff) when a macro DOES need to run to
# completion (e.g. the sdram_ctrl smoke test, which closes timing cleanly).
OL_TO="${OL_TO:-Magic.WriteLEF}"
# OL_SKIP: comma/space-separated LibreLane step ids to skip outright (passed
# as repeated `--skip`). Needed for macro-placement runs (dcache/icache):
# OpenROAD.IRDropReport unconditionally hard-fails with [PSM-0069] "Check
# connectivity failed on VDD/VSS" when a design has vendor SRAM macros whose
# power pins the default PDN generation doesn't fully strap (observed even
# with PDN_CONNECT_MACROS_TO_GRID's default of true) -- IR-drop/power
# signoff is out of scope for this routed-area exercise (same rationale as
# the OL_TO default above skipping the timing checkers), so skip it rather
# than let it block reaching Magic.WriteLEF.
OL_SKIP="${OL_SKIP:-}"

# --- 1. VHDL -> Verilog: ghdl-yosys elaborates+generic-synths the macro's
# top entity (looked up from metrics/macros.list, same mapping the synth-only
# metrics flow uses) and emits a flattened, generic-cell netlist into the
# macro's own librelane/<name>/ dir, where its config.json's
# VERILOG_FILES: ["dir::<top>.v"] expects to find it.
#
# TOP-ONLY OVERRIDE (Task 6 CAPSTONE): macro=top does NOT go through this
# shared flatten-everything netlist-gen at all. The whole point of Task 6 is
# a HIERARCHY-PRESERVING netlist -- the 8 child macros (cpu/icache_adapter/
# dcache_adapter/bootram_infer(boot_mem_gf180)/sdram_ctrl/devices/
# qspi_flash_ctrl/mem_region_mux) blackboxed BEFORE `synth -flatten`, so
# LibreLane places them as LEF macro abstracts instead of re-synthesizing
# their logic (that's what keeps top-level memory bounded -- a flat
# `-e soc; synth -top soc -flatten` with no blackbox step, which is what the
# generic per-macro path below would do for macro=top, is exactly the
# flatten-everything approach this task exists to avoid). It also needs the
# flash-variant's vendor-SRAM rebind (Task 6 PHASE A: soc.vhd's `cpus`/
# `ddr_ram_mux` bound to the gf180 architectures/configurations, not the
# base/fpga ones) and a from-scratch soc_gen regen (never the committed
# base-variant targets/asic/gf180_j4mmu/soc.vhd -- that stays byte-identical
# always, see the save/restore discipline documented in sim/xip_sim.sh).
# Additionally: this repo's HOST ghdl (6.0.0 "Dunoon") was found to crash
# (`synth-vhdl_decls.adb:302`, an internal GHDL synth-backend assertion) on
# a stale/contaminated analyze work library; a completely FRESH workdir +
# gen_synth_sources.sh's synth-clean (soc_port_*-stripped) source set +
# freshly-regenerated output/gf180_j4mmu/config/config.vhd resolved it --
# no newer GHDL build or container was actually needed in the end. The
# resulting netlist (targets/asic/gf180_j4mmu/librelane/top/soc.v, the
# top-level interconnect glue only, ~760 cells) + blackbox module stubs
# (soc_macros_bb.v, `(* blackbox *)`-marked empty declarations for the 8
# macros + the 2 leaf vendor SRAM cell types, so LibreLane's own
# Yosys.Synthesis hierarchy check resolves the instances without
# re-deriving their bodies) are PRE-GENERATED and committed alongside
# top/config.json (which points VERILOG_FILES at them via "dir::") --
# regenerating them (only if the SoC RTL changes) uses the clean-analyze recipe in
# top/README.md (bespoke to the top design's
# multi-architecture-configuration binding chain in a way the shared
# per-macro path below has no need to support for any other macro).
if [ "$MACRO" = "top" ]; then
  NETV="$MDIR/soc.v"
  if [ ! -f "$NETV" ] || [ ! -f "$MDIR/soc_macros_bb.v" ]; then
    echo "ERROR: $MDIR/soc.v and/or soc_macros_bb.v missing -- macro=top needs the" \
         "pre-generated hierarchy-preserving netlist committed alongside config.json" \
         "(see top/README.md for the" \
         "regeneration recipe)." >&2
    exit 1
  fi
  echo "run.sh: macro=top uses the pre-generated $NETV (+ soc_macros_bb.v blackbox stubs)," \
       "skipping the shared per-macro netlist-gen below." >&2
  MERGED="$MDIR/config.merged.json"
  jq -s '.[0] * .[1]' "$HERE/common.json" "$MCFG" > "$MERGED"
  # Task 6's top run always needs the PDN-checker skip (8 macros with the
  # same macro-PDN-strap gap as dcache/icache/boot_mem -- see the OL_SKIP
  # comment further below) even though "top" isn't in that case's MACRO
  # list; set it here unless the caller already did.
  if [ -z "$OL_SKIP" ]; then
    OL_SKIP="OpenROAD.IRDropReport Checker.PowerGridViolations"
  fi
  goto_librelane=1
fi
if [ -z "${goto_librelane:-}" ]; then
MACROS_LIST="$ROOT/targets/asic/gf180_j4mmu/metrics/macros.list"
ELAB_TOP=""
SYNTH_TOP=""
# Per-macro source-variant selector: "base" -> GHDL_BASE (tech/inferred
# RAM-as-flops, the default for every macro so far), "gf180_cache" ->
# GHDL_BASE_GF180_CACHE (tech/gf180 vendor-SRAM-bound cache adapters, see
# synth_gf180.sh's cache_ctrl.{i,d}cache_gf180 special case /
# gen_synth_sources.sh). Keyed on the run.sh macro= name (our librelane/dcache
# and librelane/icache directories), NOT on macros.list, since macros.list's
# rows use the metrics flow's own dotted names (cache_ctrl.dcache_gf180) and
# is shared with synth_gf180.sh/emit_gf180_metrics.py -- left untouched here.
SRC_VARIANT="base"
case "$MACRO" in
  dcache)
    ELAB_TOP="dcache_adapter_gf180"; SYNTH_TOP="dcache_adapter"; SRC_VARIANT="gf180_cache" ;;
  icache)
    ELAB_TOP="icache_adapter_gf180"; SYNTH_TOP="icache_adapter"; SRC_VARIANT="gf180_cache" ;;
  boot_mem)
    ELAB_TOP="boot_mem_top_gf180"; SYNTH_TOP="boot_mem_top_gf180"; SRC_VARIANT="gf180_bootmem" ;;
esac
if [ -z "$ELAB_TOP" ]; then
  while read -r m elab_top synth_top _rest; do
    case "$m" in ''|\#*) continue;; esac
    if [ "$m" = "$MACRO" ]; then
      ELAB_TOP="$elab_top"; SYNTH_TOP="${synth_top:-$elab_top}"
      break
    fi
  done < "$MACROS_LIST"
fi
# The smoke macro isn't a macros.list row (it IS the row it tests, named
# after its own directory); fall back to the well-known sdram_ctrl mapping.
if [ -z "$ELAB_TOP" ]; then
  if [ "$MACRO" = "smoke" ]; then
    ELAB_TOP="sdram_ctrl"; SYNTH_TOP="sdram_ctrl"
  else
    echo "ERROR: macro '$MACRO' not found in $MACROS_LIST (and is not 'smoke'/'dcache'/'icache')" >&2
    exit 1
  fi
fi

NETV="$MDIR/${SYNTH_TOP}.v"
echo "run.sh: generating $NETV (ghdl -e $ELAB_TOP; synth -top $SYNTH_TOP; src-variant=$SRC_VARIANT)" >&2
source "$ROOT/targets/asic/gf180_j4mmu/metrics/gen_synth_sources.sh"   # exports GHDL_BASE, GHDL_BASE_GF180_CACHE, GHDL_BASE_GF180_BOOTMEM
if [ "$SRC_VARIANT" = "gf180_cache" ]; then
  GHDL_CMD="$GHDL_BASE_GF180_CACHE"
elif [ "$SRC_VARIANT" = "gf180_bootmem" ]; then
  GHDL_CMD="$GHDL_BASE_GF180_BOOTMEM"
else
  GHDL_CMD="$GHDL_BASE"
fi
yosys -m ghdl -p "$GHDL_CMD -e $ELAB_TOP; synth -top $SYNTH_TOP -flatten; chformal -remove; delete t:\$check t:\$assert t:\$assume t:\$cover t:\$live t:\$fair; clean; write_verilog -noattr $NETV" \
  || { echo "ERROR: ghdl-yosys netlist generation failed for $MACRO" >&2; exit 1; }
if [ "$SRC_VARIANT" = "gf180_cache" ] || [ "$SRC_VARIANT" = "gf180_bootmem" ]; then
  if ! grep -q "gf180mcu_fd_ip_sram__sram512x8m8wm1" "$NETV"; then
    echo "ERROR: $MACRO netlist does not instantiate the vendor sram512x8 macro" \
         "(data RAM regressed to inferred flops?) -- see $NETV" >&2
    exit 1
  fi
fi
# Sanitize VHDL-record-flattened port names: \name[field]  ->  name_field
perl -0pe 's/\\([A-Za-z_][A-Za-z0-9_]*)\[([A-Za-z0-9_]+)\]([ \t]|(?=\n))/${1}_${2}$3/g' \
  "$NETV" > "$NETV.tmp" && mv "$NETV.tmp" "$NETV"

# --- 2. Merge common.json (shared PDK/clock/util defaults) over the macro's
# config.json (macro-specific DESIGN_NAME/VERILOG_FILES/clock override).
MERGED="$MDIR/config.merged.json"
jq -s '.[0] * .[1]' "$HERE/common.json" "$MCFG" > "$MERGED"
fi
# (macro=top's TOP-ONLY OVERRIDE branch above already computed MERGED and
# jumps straight here.)

# --- 3. Run LibreLane under a hard wall-clock cap (same watchdog pattern as
# components/cpu/synth/openlane/run.sh: `timeout` on the docker CLIENT does
# not stop the container, so name it and `docker kill` on expiry).
TO_ARGS=()
[ -n "$OL_TO" ] && TO_ARGS=(--to "$OL_TO")
# dcache/icache carry vendor SRAM macros whose power pins the default PDN
# generation doesn't fully strap (see OL_SKIP comment above) -- skip the
# IR-drop signoff step for them unless the caller already set OL_SKIP.
if [ -z "$OL_SKIP" ] && { [ "$MACRO" = "dcache" ] || [ "$MACRO" = "icache" ] || [ "$MACRO" = "boot_mem" ]; }; then
  # Checker.PowerGridViolations is also skipped: it's a *deferred* checker
  # (collects issues, only raised at the very end of the run regardless of
  # --to) that fires on the same macro-PDN-strap gap as IRDropReport above
  # (816178 "power grid violations (... you may ignore these if LVS
  # passes)"). Deferred errors are re-raised at flow end even after the
  # requested --to stop point has already completed successfully (LEF/GDS
  # for dcache_adapter were written to runs/smoke/52-magic-writelef/ before
  # this error surfaces) -- skipping it is required to get a clean exit
  # code and a final/ views directory out of the run.
  OL_SKIP="OpenROAD.IRDropReport Checker.PowerGridViolations"
fi
for s in $OL_SKIP; do
  TO_ARGS+=(--skip "$s")
done

USE_DOCKER=0
if command -v librelane >/dev/null 2>&1; then
  USE_DOCKER=0
elif command -v docker >/dev/null 2>&1; then
  USE_DOCKER=1
else
  echo "ERROR: no librelane binary and no docker on PATH" >&2
  exit 1
fi

rc=0
if [ "$USE_DOCKER" -eq 1 ]; then
  CNAME="librelane-${MACRO}-$$"
  docker rm -f "$CNAME" >/dev/null 2>&1 || true
  ( sleep "$OL_TIMEOUT"
    echo "WARN: ${OL_TIMEOUT}s wall-clock cap hit -- killing container $CNAME" >&2
    docker kill "$CNAME" >/dev/null 2>&1 ) &
  WD=$!
  docker run --rm --name "$CNAME" \
    -v "$ROOT:$ROOT" -v "$PDK_ROOT:$PDK_ROOT" -w "$ROOT" \
    -e PDK_ROOT="$PDK_ROOT" \
    "$OL_IMAGE" librelane --manual-pdk --pdk-root "$PDK_ROOT" --overwrite --run-tag smoke "${TO_ARGS[@]}" "$MERGED" || rc=$?
  kill "$WD" >/dev/null 2>&1 || true
  wait "$WD" 2>/dev/null || true
else
  timeout --kill-after=60 "$OL_TIMEOUT" \
    librelane --manual-pdk --pdk-root "$PDK_ROOT" --overwrite --run-tag smoke "${TO_ARGS[@]}" "$MERGED" || rc=$?
fi

RUNDIR="$(find "$MDIR/runs" -maxdepth 1 -name 'smoke*' 2>/dev/null | xargs -r ls -dt 2>/dev/null | head -1 || true)"
echo "run.sh: rc=$rc run dir=${RUNDIR:-<none>}"
if [ -n "$RUNDIR" ]; then
  FINAL_METRICS="$RUNDIR/final/metrics.json"
  if [ -f "$FINAL_METRICS" ]; then
    echo "run.sh: metrics -> $FINAL_METRICS"
  else
    LATEST="$(find "$RUNDIR" -name metrics.json 2>/dev/null | xargs -r ls -t 2>/dev/null | head -1 || true)"
    echo "run.sh: no final/metrics.json (stopped early?); latest step metrics -> ${LATEST:-<none>}"
  fi
fi
exit "$rc"
