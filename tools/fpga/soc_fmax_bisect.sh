#!/usr/bin/env bash
# soc_fmax_bisect.sh -- `git bisect run` probe for the ULX3S SoC Fmax regression.
#
# Run from the jcore-cpu submodule under `git bisect run`, but builds in
# jcore-soc: bisect checks out a cpu commit, we measure the whole SoC at that
# pin and answer good/bad against a threshold.
#
# Usage (from the repo root, after starting bisect in components/cpu):
#   VARIANT=j4-rom THRESHOLD=28.0 SEEDS=1 tools/fpga/soc_fmax_bisect.sh
#
# Exit codes are git-bisect's: 0 = good (fast), 1 = bad (slow), 125 = skip
# (cannot build this commit -- bisect then tries a neighbour rather than
# recording a false verdict).
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"

VARIANT="${VARIANT:?VARIANT must be set}"
THRESHOLD="${THRESHOLD:?THRESHOLD (MHz) must be set}"
SEEDS="${SEEDS:-1}"

SHA="$(git -C components/cpu rev-parse HEAD)"

vals=()
for s in $(seq 1 "$SEEDS"); do
  if line="$(tools/fpga/soc_fmax.sh "$SHA" "$VARIANT" "$s" 2>/dev/null)"; then
    vals+=("$(printf '%s' "$line" | python3 -c 'import json,sys; print(json.load(sys.stdin)["fmax"])')")
  else
    echo "bisect: $SHA does not build -- skipping" >&2
    exit 125
  fi
done

MED="$(python3 - "${vals[@]}" <<'PY'
import statistics, sys
print("%.2f" % statistics.median([float(v) for v in sys.argv[1:]]))
PY
)"

echo "bisect: $SHA $VARIANT median ${MED} MHz (threshold ${THRESHOLD})" >&2
python3 -c "import sys; sys.exit(0 if float('$MED') >= float('$THRESHOLD') else 1)"
