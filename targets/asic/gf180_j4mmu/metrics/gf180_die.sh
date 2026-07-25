#!/usr/bin/env bash
# GF180 padded-die P&R driver: hardens the 8 macros + the integrated `top`
# through LibreLane place & route, then emits the canonical die-area metrics
# doc (tools/asic/emit_die_metrics.py) consumed by the board-synth.yml
# `gf180-die-area` job / dashboard pipeline.
#
# Assumes the CALLER has already done `ciel enable --pdk-family gf180mcu
# $PDK_PIN` (this script does not manage the PDK checkout -- same division
# of labor as librelane/run.sh, which just expects PDK_ROOT to be populated).
#
# THE `top` GDS-MERGE LIMITATION (read before touching the "top" branch
# below): `librelane/run.sh macro=top` runs the full Classic flow to
# completion (unlike the other 8 macros, which stop at Magic.WriteLEF --
# see run.sh's OL_TO comment) because `top` needs a real GDS for its final
# finishing/merge step. That finishing step (streaming out + merging the
# blackboxed macros' GDS into the top GDS) is KNOWN TO FAIL (see the Task 7
# report: the 8 macros only have LEF abstracts committed here, not GDS --
# GDS merge needs the real macro GDS, which is a separate, heavier
# artifact this sub-project never asked LibreLane to keep). So `run.sh
# macro=top` reliably exits non-zero at that LAST step, AFTER routing +
# parasitic extraction (RCX) have already completed and written their
# `or_metrics_out.json` -- i.e. the routed-area numbers we actually want
# (die/core/instance area) are already on disk by the time the run fails.
# We therefore do NOT treat `run.sh macro=top`'s exit code as fatal: we
# always try to locate the LAST successful step's `or_metrics_out.json`
# under runs/*/ (preferring `final/metrics.json` if the run did complete
# cleanly, e.g. after a future fix to the GDS-merge step) and feed that to
# emit_die_metrics.py regardless of run.sh's rc.
set -uo pipefail
ROOT="$(cd "$(dirname "$0")/../../../.." && pwd)"
cd "$ROOT"

LIBRELANE_DIR="targets/asic/gf180_j4mmu/librelane"
OUT_DIR="${OUT_DIR:-metrics-die}"
COMMIT="${COMMIT:-$(git rev-parse HEAD 2>/dev/null || echo unknown)}"

# macro name -> librelane/<dir> (the "cpu"/"devices"/"sdram_ctrl" aliases
# from the task brief map to the actual librelane subdirectory names).
declare -A MACRO_DIR=(
  [j4_core]=j4_core
  [icache]=icache
  [dcache]=dcache
  [boot_mem]=boot_mem
  [sdram_ctrl]=smoke
  [devices]=soc_cluster.devices
  [qspi_flash]=qspi_flash
  [mem_region_mux]=mem_region_mux
)
MACRO_ORDER=(j4_core icache dcache boot_mem sdram_ctrl devices qspi_flash mem_region_mux)

apply_pdk_overlay() {
  "$LIBRELANE_DIR/pdk_overlay/apply.sh" || {
    echo "WARN: PDK overlay apply.sh failed/non-idempotent -- continuing" >&2
  }
}

# Return the newest or_metrics_out.json (by step number prefix, e.g.
# "48-openroad-rcx" sorts after "38-...") for a completed or partially
# completed run dir, preferring runs/*/final/metrics.json when present.
latest_metrics_json() {
  local run_root="$1"
  local final
  final=$(find "$run_root" -path '*/final/metrics.json' 2>/dev/null | head -1)
  if [ -n "$final" ]; then
    echo "$final"
    return 0
  fi
  find "$run_root" -name 'or_metrics_out.json' 2>/dev/null \
    | sort -t/ -k2 -V \
    | tail -1
}

echo "=== gf180_die.sh: applying PDK overlay ==="
apply_pdk_overlay

MACRO_ARGS=()
for name in "${MACRO_ORDER[@]}"; do
  dir="${MACRO_DIR[$name]}"
  echo "=== gf180_die.sh: hardening macro=$dir (die series name: $name) ==="
  ( "$LIBRELANE_DIR/run.sh" macro="$dir" )
  rc=$?
  if [ $rc -ne 0 ]; then
    echo "WARN: run.sh macro=$dir exited $rc -- looking for partial metrics anyway" >&2
  fi
  m="$(latest_metrics_json "$LIBRELANE_DIR/$dir/runs")"
  if [ -n "$m" ] && [ -f "$m" ]; then
    echo "  -> metrics: $m"
    MACRO_ARGS+=(--macro "$name=$m")
  else
    echo "WARN: no metrics.json found for macro=$dir -- omitting from die doc" >&2
  fi
  # Disk-space discipline: LibreLane's per-step run dirs + docker layers add
  # up fast on the ~14GB github-hosted runner; reclaim between macros.
  docker system prune -f >/dev/null 2>&1 || true
done

echo "=== gf180_die.sh: hardening/routing macro=top (integrated padded die) ==="
TOP_ARGS=()
( "$LIBRELANE_DIR/run.sh" macro=top )
rc=$?
if [ $rc -ne 0 ]; then
  echo "WARN: run.sh macro=top exited $rc -- this is the KNOWN GDS-merge" \
       "finishing-step failure (see this script's header comment); routing" \
       "should have completed before that step. Extracting routed area from" \
       "the last successful step instead of final/metrics.json." >&2
fi
top_m="$(latest_metrics_json "$LIBRELANE_DIR/top/runs")"
if [ -n "$top_m" ] && [ -f "$top_m" ]; then
  echo "  -> top metrics: $top_m"
  TOP_ARGS=(--top "$top_m")
else
  echo "WARN: no metrics.json found for macro=top -- die/core/placed-silicon" \
       "series will be omitted, per-macro series + kianv constant still emitted" >&2
fi
docker system prune -f >/dev/null 2>&1 || true

echo "=== gf180_die.sh: emitting canonical die metrics ==="
mkdir -p "$OUT_DIR"
python3 tools/asic/emit_die_metrics.py \
  "${TOP_ARGS[@]}" \
  "${MACRO_ARGS[@]}" \
  --commit "$COMMIT" \
  --out "$OUT_DIR/metrics-die.json"

echo "=== gf180_die.sh: done -- $OUT_DIR/metrics-die.json ==="
cat "$OUT_DIR/metrics-die.json"
# Always exit 0: a partial P&R (missing macro or missing top) should still
# publish whatever metrics WERE produced, not fail the CI job outright --
# the job's own steps are individually guarded (continue-on-error/if:
# always()) for the same reason.
exit 0
