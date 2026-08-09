#!/usr/bin/env bash
# GF180 whole-chip P&R benchmark: hardens the COMPLETE chip -- the flat
# flash-variant `soc` plus an abutted gf180mcu_fd_io pad ring -- through
# LibreLane's Chip flow, and emits the canonical die-area metrics doc consumed
# by the board-synth.yml `gf180-die-area` job / dashboard pipeline.
#
# Flow:
#   1. regenerate chip_core.v (six child netlists + top/soc.v glue, flattened;
#      ~60s, no LibreLane -- see chip_core/gen_chip_core.sh);
#   2. harden chip_top (run.sh macro=chip_top), the single whole-chip P&R;
#   3. collect chip_top's layout render as a CI artifact;
#   4. emit metrics-die.json (padded die area + DRC + the pinned KianV line).
#
# This REPLACES the six-macro harden + flat `pad_ring` assembly. That path
# hand-placed each child macro at fixed coordinates in top/config.json, and the
# J4 sh4-overlay decoder grew `cpus` to 1661x1688 um -- overlapping `devices`
# by 344x715 um and `icache_adapter` by 1557x368 um, which broke the PDN
# (PSM-0069) and diverged global placement (GPL-0305). chip_top routes the same
# design DRC-clean at 12.92 mm2 with no hand placement at all. See
# targets/asic/gf180_j4mmu/librelane/chip_top/README.md and
# docs/asic/gf180-vs-kianv-comparison.md.
#
# Assumes the CALLER already did `ciel enable --pdk-family gf180mcu $PDK_PIN`.
# Every leg is guarded: a partial build still publishes whatever was produced
# (the kianv line is always emitted). The publish path therefore always runs to
# completion -- but the script EXITS NON-ZERO at the end if nothing real was
# built, so a broken flow cannot masquerade as a green nightly.
#
# WHY (2026-08-08): this script used to `exit 0` unconditionally, under a
# `continue-on-error: true` workflow step. From the LibreLane 3.0.5 bump
# (5c5e431, 2026-07-31) every macro leg failed at STA Pre-PnR and the job still
# reported success for nine days -- no die area was ever produced, and the
# post-J4-decoder design was never place-and-routed. Deferred-exit (rather than
# `set -e`) keeps the partial-publish behaviour that motivated the original
# exit 0, without the dishonesty.
set -uo pipefail

# Deferred failure flag -- see the header. Set by any leg that produced nothing
# real; consumed by the exit at the very bottom, after all publishing is done.
DIE_FAILED=0
ROOT="$(cd "$(dirname "$0")/../../../.." && pwd)"
cd "$ROOT"

LIBRELANE_DIR="targets/asic/gf180_j4mmu/librelane"
OUT_DIR="${OUT_DIR:-metrics-die}"
COMMIT="${COMMIT:-$(git rev-parse HEAD 2>/dev/null || echo unknown)}"

# Wall-clock cap handed to run.sh (its own default is 3600s), which the single
# chip_top P&R needs: measured 2026-08-09 at 7280s (2h01m) through detailed
# routing on a developer workstation -- 75,661 instances, 12.92 mm2, 0 route
# DRC. 3h leaves headroom for a slower CI runner; GitHub's 6h job limit is the
# backstop, and one chip_top run fits it far more comfortably than the six
# macro legs plus pad ring this replaced.
export OL_TIMEOUT="${OL_TIMEOUT:-10800}"

latest_metrics_json() {
  local run_root="$1" final
  final=$(find "$run_root" -path '*/final/metrics.json' 2>/dev/null | head -1)
  if [ -n "$final" ]; then echo "$final"; return 0; fi
  find "$run_root" -name 'or_metrics_out.json' 2>/dev/null | sort -t/ -k2 -V | tail -1
}

echo "=== gf180_die.sh: applying PDK overlay ==="
"$LIBRELANE_DIR/pdk_overlay/apply.sh" || echo "WARN: PDK overlay apply.sh non-idempotent -- continuing" >&2

# --- 1. regenerate the flat soc netlist ----------------------------------
# The six child netlists + top/soc.v glue, flattened into chip_core.v (one
# `soc` module, 17 vendor SRAMs as blackbox leaves). ~60s: OL_NETLIST_ONLY
# skips LibreLane entirely for the children -- nothing places them any more.
# This also stops chip_core.v going stale, which is how a pre-J4-decoder
# netlist silently survived from 2026-07-31 to 2026-08-09.
echo "=== gf180_die.sh: regenerating chip_core.v (child netlists + flatten) ==="
if ! REGEN_CHILDREN=1 "$LIBRELANE_DIR/chip_core/gen_chip_core.sh"; then
  echo "WARN: gen_chip_core.sh failed" >&2
  DIE_FAILED=1
fi

# --- 2. harden the whole chip --------------------------------------------
# chip_top = LibreLane Chip flow: the flat soc plus an ABUTTED gf180mcu_fd_io
# pad ring, in one P&R. Replaces the old six-macro harden + pad_ring
# assembly, whose hand-placed floorplan the J4 decoder outgrew (cpus reached
# 1661x1688 um and overlapped devices and icache_adapter, breaking the PDN
# and diverging global placement).
echo "=== gf180_die.sh: hardening chip_top (flat soc + abutted pad ring) ==="
# Stamp the wall clock BEFORE the run so a metrics.json left behind by an
# EARLIER run cannot be published as this run's result. On a fresh CI checkout
# there is no stale run directory, but on a dev box or a self-hosted runner
# with a persistent workspace there is -- and a failed run that republishes
# yesterday's die area while exiting 0 is exactly the false-green this script
# exists to prevent. (Caught by this script's own negative test.)
CHIPTOP_STAMP="$LIBRELANE_DIR/chip_top/.die_run_stamp"
: > "$CHIPTOP_STAMP"
CHIPTOP_RC=0
( "$LIBRELANE_DIR/run.sh" macro=chip_top ) || CHIPTOP_RC=$?
if [ "$CHIPTOP_RC" -ne 0 ]; then
  echo "WARN: run.sh macro=chip_top exited $CHIPTOP_RC" >&2
  DIE_FAILED=1
fi

CHIPTOP_ARG=()
CHIPTOP_JSON="$(latest_metrics_json "$LIBRELANE_DIR/chip_top/runs")"
if [ -z "$CHIPTOP_JSON" ] || [ ! -f "$CHIPTOP_JSON" ]; then
  echo "WARN: chip_top produced no metrics.json -- no die was built" >&2
  DIE_FAILED=1
elif [ ! "$CHIPTOP_JSON" -nt "$CHIPTOP_STAMP" ]; then
  echo "WARN: $CHIPTOP_JSON predates this run -- refusing to publish a stale" \
       "die area from an earlier run" >&2
  DIE_FAILED=1
else
  echo "  -> metrics: $CHIPTOP_JSON"
  CHIPTOP_ARG=(--chip-top "$CHIPTOP_JSON")
fi
rm -f "$CHIPTOP_STAMP"

# --- 3. collect the layout render (CI artifact) --------------------------
# chip_top's own KLayout.Render step writes the layout view into its run
# directory during signoff; copy out whatever it produced. (The old flow
# rendered a DEF by hand via pad_ring/render_def.py.)
mkdir -p "$OUT_DIR"
CHIPTOP_PNG="$(find "$LIBRELANE_DIR/chip_top/runs" -name '*.png' 2>/dev/null | head -1)"
if [ -n "$CHIPTOP_PNG" ]; then
  echo "=== gf180_die.sh: collecting layout render $CHIPTOP_PNG ==="
  cp "$CHIPTOP_PNG" "$OUT_DIR/gf180-padded-die.png" || echo "WARN: render copy failed" >&2
else
  echo "note: chip_top produced no .png (run stopped before KLayout.Render)" >&2
fi

# --- 4. emit canonical die metrics ---------------------------------------
echo "=== gf180_die.sh: emitting canonical die metrics ==="
python3 tools/asic/emit_die_metrics.py \
  "${CHIPTOP_ARG[@]}" \
  --commit "$COMMIT" \
  --out "$OUT_DIR/metrics-die.json"

echo "=== gf180_die.sh: done -- $OUT_DIR/metrics-die.json ==="
cat "$OUT_DIR/metrics-die.json"

# Everything above has published whatever was produced. NOW report the truth.
if [ "$DIE_FAILED" -ne 0 ]; then
  echo "ERROR: gf180_die.sh: the die flow did not complete -- see the WARN" \
       "lines above for which stage produced no metrics. The dashboard doc" \
       "was still emitted so the series stays" \
       "alive, but this run built nothing real." >&2
  exit 1
fi
exit 0
