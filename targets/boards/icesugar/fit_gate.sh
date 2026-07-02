#!/usr/bin/env bash
# Fit + timing gate for the iCESugar UP5K build. Exits nonzero when the J1 does
# NOT fit (nextpnr did not place -> no bitstream, or ICESTORM_LC over the 5280
# budget) or a constrained clock misses its declared frequency at FINAL
# (post-route) timing. The timing check keeps the LAST verdict per clock, so an
# intermediate nextpnr estimate cannot false-positive (same rule as ulx3s).
# Usage: fit_gate.sh <nextpnr.log> <bitstream-file>
set -uo pipefail
LOG="${1:?usage: fit_gate.sh <nextpnr.log> <bitstream-file>}"
BIT="${2:?usage: fit_gate.sh <nextpnr.log> <bitstream-file>}"
BUDGET_LC=5280
fail=0

# Fit — bitstream present (nextpnr aborts without one when it cannot place).
if [ ! -f "$BIT" ]; then
  echo "FIT GATE: no bitstream ($BIT) — nextpnr did not place (over UP5K budget)" >&2
  fail=1
fi

# Fit — explicit ICESTORM_LC budget (nextpnr prints the count even on failure;
# take the LAST occurrence = final).
lc=$(grep -oE "ICESTORM_LC:[[:space:]]*[0-9]+" "$LOG" 2>/dev/null | tail -1 | grep -oE "[0-9]+$" || true)
if [ -n "$lc" ] && [ "$lc" -gt "$BUDGET_LC" ]; then
  echo "FIT GATE: ICESTORM_LC $lc > $BUDGET_LC (does not fit UP5K)" >&2
  fail=1
fi

# Timing — fail only if a clock's FINAL verdict is FAIL.
if ! awk '
  /Max frequency for clock/ {
    line = $0; sub(/.*clock '\''/, "", line); sub(/'\''.*/, "", line)
    last[line] = ($0 ~ /\(FAIL at/) ? "FAIL" : "PASS"
  }
  END { bad = 0; for (c in last) if (last[c] == "FAIL") { print "  " c > "/dev/stderr"; bad = 1 } exit bad }
' "$LOG"; then
  echo "TIMING GATE: a constrained clock misses its declared frequency at final timing" >&2
  fail=1
fi

if [ "$fail" -ne 0 ]; then
  echo "icesugar: fit/timing gate FAILED (see $LOG)" >&2
  exit 1
fi
echo "icesugar: fit + timing OK (ICESTORM_LC ${lc:-?}/$BUDGET_LC; all constrained clocks meet timing)"
