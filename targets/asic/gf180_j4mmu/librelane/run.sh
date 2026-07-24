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
OL_TO="${OL_TO:-}"

# --- 1. VHDL -> Verilog: ghdl-yosys elaborates+generic-synths the macro's
# top entity (looked up from metrics/macros.list, same mapping the synth-only
# metrics flow uses) and emits a flattened, generic-cell netlist into the
# macro's own librelane/<name>/ dir, where its config.json's
# VERILOG_FILES: ["dir::<top>.v"] expects to find it.
MACROS_LIST="$ROOT/targets/asic/gf180_j4mmu/metrics/macros.list"
ELAB_TOP=""
SYNTH_TOP=""
while read -r m elab_top synth_top _rest; do
  case "$m" in ''|\#*) continue;; esac
  if [ "$m" = "$MACRO" ]; then
    ELAB_TOP="$elab_top"; SYNTH_TOP="${synth_top:-$elab_top}"
    break
  fi
done < "$MACROS_LIST"
# The smoke macro isn't a macros.list row (it IS the row it tests, named
# after its own directory); fall back to the well-known sdram_ctrl mapping.
if [ -z "$ELAB_TOP" ]; then
  if [ "$MACRO" = "smoke" ]; then
    ELAB_TOP="sdram_ctrl"; SYNTH_TOP="sdram_ctrl"
  else
    echo "ERROR: macro '$MACRO' not found in $MACROS_LIST (and is not 'smoke')" >&2
    exit 1
  fi
fi

NETV="$MDIR/${SYNTH_TOP}.v"
echo "run.sh: generating $NETV (ghdl -e $ELAB_TOP; synth -top $SYNTH_TOP)" >&2
source "$ROOT/targets/asic/gf180_j4mmu/metrics/gen_synth_sources.sh"   # exports GHDL_BASE
yosys -m ghdl -p "$GHDL_BASE -e $ELAB_TOP; synth -top $SYNTH_TOP -flatten; write_verilog -noattr $NETV" \
  || { echo "ERROR: ghdl-yosys netlist generation failed for $MACRO" >&2; exit 1; }
# Sanitize VHDL-record-flattened port names: \name[field]  ->  name_field
perl -0pe 's/\\([A-Za-z_][A-Za-z0-9_]*)\[([A-Za-z0-9_]+)\]([ \t]|(?=\n))/${1}_${2}$3/g' \
  "$NETV" > "$NETV.tmp" && mv "$NETV.tmp" "$NETV"

# --- 2. Merge common.json (shared PDK/clock/util defaults) over the macro's
# config.json (macro-specific DESIGN_NAME/VERILOG_FILES/clock override).
MERGED="$MDIR/config.merged.json"
jq -s '.[0] * .[1]' "$HERE/common.json" "$MCFG" > "$MERGED"

# --- 3. Run LibreLane under a hard wall-clock cap (same watchdog pattern as
# components/cpu/synth/openlane/run.sh: `timeout` on the docker CLIENT does
# not stop the container, so name it and `docker kill` on expiry).
TO_ARGS=()
[ -n "$OL_TO" ] && TO_ARGS=(--to "$OL_TO")

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
