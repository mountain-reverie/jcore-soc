#!/usr/bin/env bash
# soc_fmax.sh -- real ULX3S SoC ECP5 Fmax for one jcore-cpu commit.
#
# Pins components/cpu to <cpu-sha>, regenerates the SoC for <variant>, builds
# the bitstream, and reports Fmax plus the binding critical path.
#
# The jcore-soc tree is held FIXED; only the submodule pin moves. Anything else
# varying between two points would confound the comparison.
#
# Usage: tools/fpga/soc_fmax.sh <cpu-sha> <variant> [seed]
#   variant: j2-direct | j2-dual | j4-dual | j4-rom
# Output: one JSON line on stdout, also appended to build/soc_fmax.jsonl
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"

[ $# -ge 2 ] || { echo "usage: tools/fpga/soc_fmax.sh <cpu-sha> <variant> [seed]" >&2; exit 2; }
CPU_SHA="$1"
VARIANT="$2"
SEED_IN="${3:-}"

case "$VARIANT" in
  j2-direct|j2-dual|j4-dual|j4-rom) ;;
  *) echo "unknown variant: $VARIANT" >&2; exit 2 ;;
esac

fail() { echo "ERROR: cpu=$CPU_SHA variant=$VARIANT seed=${SEED_IN:-default}: $1" >&2; exit 1; }

# The build process (ghdl/synth) mutates tracked files in-tree on some
# commits (observed: core/datapath.vhd). Left dirty, that blocks the NEXT
# `git checkout` -- including one done outside this script, e.g. by
# `git bisect run`'s own advance to the next commit after we exit. Always
# discard it, on every exit path (success, fail, or skip).
cleanup() { git -C components/cpu checkout --quiet -- . 2>/dev/null || true; }
trap cleanup EXIT

# 1. Pin the submodule. --detach so we never move a branch under the user.
git -C components/cpu checkout --quiet -- . 2>/dev/null || true
git -C components/cpu fetch --quiet origin "$CPU_SHA" 2>/dev/null || true
git -C components/cpu checkout --quiet --detach "$CPU_SHA" || fail "cannot check out cpu sha"

# 2. Regenerate the SoC for this variant. socgen emits cpus_config.vhd and
#    cpu_synth_files.list; synth.sh copies them verbatim, so skipping this would
#    silently synthesize the previously generated variant.
make ulx3s TARGET=soc_gen VARIANT="$VARIANT" >/dev/null 2>&1 || fail "soc_gen failed"

# 3. Build. synth.sh gates timing by exiting non-zero AFTER writing metrics.json,
#    so a timing miss must not abort us -- we are measuring, not gating.
#    NOTE: synth.sh does `rm -rf "$OUT"; mkdir -p "$OUT"` on its own build dir,
#    so the log MUST live outside that directory or it gets deleted out from
#    under the redirect while it is being written.
OUT="targets/boards/ulx3s/build"
mkdir -p build
BUILD_LOG="build/soc_fmax_build.log"
set +e
if [ -n "$SEED_IN" ]; then
  SEED="$SEED_IN" VARIANT="$VARIANT" ./targets/boards/ulx3s/synth.sh >"$BUILD_LOG" 2>&1
else
  VARIANT="$VARIANT" ./targets/boards/ulx3s/synth.sh >"$BUILD_LOG" 2>&1
fi
set -e

[ -f "$OUT/metrics.json" ] || { tail -40 "$BUILD_LOG" >&2; fail "no metrics.json produced"; }

# 4. Extract. A metrics.json with no Fmax means the build did not really finish.
EXTRACT="$(python3 tools/fpga/soc_fmax.py --metrics "$OUT/metrics.json" --json)" \
  || { tail -40 "$BUILD_LOG" >&2; fail "metrics.json has no usable Fmax"; }

LINE="$(python3 - "$CPU_SHA" "$VARIANT" "${SEED_IN:-default}" "$EXTRACT" <<'PY'
import json, sys
sha, variant, seed, blob = sys.argv[1:5]
d = json.loads(blob)
print(json.dumps({"cpu_sha": sha, "variant": variant, "seed": seed,
                  "fmax": d["fmax"], "critical_path": d["critical_path"]}))
PY
)"

mkdir -p build
cp "$OUT/metrics.json" "build/metrics-${CPU_SHA}-${VARIANT}-${SEED_IN:-default}.json"
printf '%s\n' "$LINE" | tee -a build/soc_fmax.jsonl
