#!/usr/bin/env bash
# GF180 padded-die P&R benchmark: builds the COMPLETE chip -- the flash-variant
# `soc` (2 KB caches) wrapped in a KianV-style gf180mcu_fd_io pad ring, routed
# FLAT (soc children black-boxed, glue + pads in one domain) -- and emits the
# canonical die-area metrics doc consumed by the board-synth.yml
# `gf180-die-area` job / dashboard pipeline.
#
# This REPLACES the earlier soc-as-macro top flow (core-only, no pad ring,
# 21.2 mm2): the flat integration produces a full padded die at 17.5 mm2,
# 0 DRC -- below KianV's 20.1 mm2 reference -- and is faster. See
# targets/asic/gf180_j4mmu/librelane/pad_ring/README.md and
# docs/asic/gf180-vs-kianv-comparison.md.
#
# Flow:
#   1. harden the 6 soc child macros to LEF (their footprints feed the flat
#      pad_ring config's MACROS placement);
#   2. generate the pad-ring netlist + flat config (gen_netlist.py/gen_config.py);
#   3. harden pad_ring through placement + CTS (run.sh macro=pad_ring);
#   4. finish the route with direct OpenROAD (finish_route.sh -> route.tcl):
#      global_route -allow_congestion past GRT-0118, PAD/power nets special;
#   5. render the routed DEF to a PNG (attached as a CI artifact);
#   6. emit metrics-die.json (padded die area + DRC + per-macro areas + the
#      pinned KianV limit line).
#
# Assumes the CALLER already did `ciel enable --pdk-family gf180mcu $PDK_PIN`.
# Every leg is guarded: a partial build still publishes whatever was produced
# (the kianv line is always emitted) -- so exit 0 at the end regardless.
set -uo pipefail
ROOT="$(cd "$(dirname "$0")/../../../.." && pwd)"
cd "$ROOT"

LIBRELANE_DIR="targets/asic/gf180_j4mmu/librelane"
PADRING_DIR="$LIBRELANE_DIR/pad_ring"
OUT_DIR="${OUT_DIR:-metrics-die}"
COMMIT="${COMMIT:-$(git rev-parse HEAD 2>/dev/null || echo unknown)}"

# The 6 flat-integration child macros: run.sh <dir> -> die-series <name>.
# (gen_config.py maps these dirs to the soc instance LEFs.)
declare -A MACRO_DIR=(
  [cpus]=cpus
  [icache_2k]=icache_2k
  [dcache_2k]=dcache_2k
  [sdram_ctrl]=smoke
  [devices]=soc_cluster.devices
  [qspi_flash]=qspi_flash
)
MACRO_ORDER=(cpus icache_2k dcache_2k sdram_ctrl devices qspi_flash)

latest_metrics_json() {
  local run_root="$1" final
  final=$(find "$run_root" -path '*/final/metrics.json' 2>/dev/null | head -1)
  if [ -n "$final" ]; then echo "$final"; return 0; fi
  find "$run_root" -name 'or_metrics_out.json' 2>/dev/null | sort -t/ -k2 -V | tail -1
}

echo "=== gf180_die.sh: applying PDK overlay ==="
"$LIBRELANE_DIR/pdk_overlay/apply.sh" || echo "WARN: PDK overlay apply.sh non-idempotent -- continuing" >&2

# --- 1. harden the 6 child macros to LEF ---------------------------------
MACRO_ARGS=()
for name in "${MACRO_ORDER[@]}"; do
  dir="${MACRO_DIR[$name]}"
  echo "=== gf180_die.sh: hardening child macro=$dir (die series: $name) ==="
  ( "$LIBRELANE_DIR/run.sh" macro="$dir" ) || echo "WARN: run.sh macro=$dir non-zero" >&2
  m="$(latest_metrics_json "$LIBRELANE_DIR/$dir/runs")"
  if [ -n "$m" ] && [ -f "$m" ]; then
    echo "  -> metrics: $m"; MACRO_ARGS+=(--macro "$name=$m")
  else
    echo "WARN: no metrics.json for macro=$dir -- omitting from die doc" >&2
  fi
  docker system prune -f >/dev/null 2>&1 || true
done

# --- 2. generate the pad-ring netlist + flat config ----------------------
echo "=== gf180_die.sh: generating pad ring netlist + flat config ==="
python3 "$PADRING_DIR/gen_netlist.py" || echo "WARN: gen_netlist.py failed" >&2
python3 "$PADRING_DIR/gen_config.py"  || echo "WARN: gen_config.py failed" >&2

# --- 3. harden pad_ring through placement + CTS --------------------------
echo "=== gf180_die.sh: hardening pad_ring (flat soc + pads) through CTS ==="
( "$LIBRELANE_DIR/run.sh" macro=pad_ring ) || echo "WARN: run.sh macro=pad_ring non-zero" >&2

# --- 4. finish the route with direct OpenROAD ----------------------------
echo "=== gf180_die.sh: finishing route with direct OpenROAD ==="
( "$PADRING_DIR/finish_route.sh" ) || echo "WARN: finish_route.sh non-zero" >&2
PADRING_JSON="$PADRING_DIR/runs/padring_metrics.json"
ROUTED_DEF="$PADRING_DIR/runs/routed_flat.def"

# --- 5. render the routed DEF to a PNG (CI artifact) ---------------------
mkdir -p "$OUT_DIR"
if [ -f "$ROUTED_DEF" ]; then
  echo "=== gf180_die.sh: rendering padded-die PNG ==="
  python3 "$PADRING_DIR/render_def.py" "$ROUTED_DEF" "$PADRING_DIR/config.json" \
    "$OUT_DIR/gf180-padded-die.png" || echo "WARN: render_def.py failed" >&2
fi

# --- 6. emit canonical die metrics ---------------------------------------
echo "=== gf180_die.sh: emitting canonical die metrics ==="
PADDED_ARG=()
[ -f "$PADRING_JSON" ] && PADDED_ARG=(--padded-die "$PADRING_JSON")
python3 tools/asic/emit_die_metrics.py \
  "${PADDED_ARG[@]}" \
  "${MACRO_ARGS[@]}" \
  --commit "$COMMIT" \
  --out "$OUT_DIR/metrics-die.json"

echo "=== gf180_die.sh: done -- $OUT_DIR/metrics-die.json ==="
cat "$OUT_DIR/metrics-die.json"
exit 0
