#!/usr/bin/env bash
# Unit test for fit_gate.sh: feed fixture nextpnr logs + a present/absent
# bitstream and assert the gate's exit code.
set -uo pipefail
HERE="$(cd "$(dirname "$0")" && pwd)"
GATE="$HERE/../fit_gate.sh"
FIX="$HERE/fixtures"
BIT_PRESENT="$(mktemp)"      # a file that exists  -> "bitstream produced"
BIT_ABSENT="$FIX/nonexistent.bin"  # never created -> "did not place"
fails=0

check() {  # <desc> <expected-rc> <log> <bitfile>
  "$GATE" "$3" "$4" >/dev/null 2>&1; local rc=$?
  if [ "$rc" -ne "$2" ]; then echo "FAIL: $1 (rc=$rc want $2)"; fails=$((fails+1));
  else echo "ok: $1"; fi
}

check "in-budget + PASS -> 0"                 0 "$FIX/pass.log"                     "$BIT_PRESENT"
check "over-budget LC -> 1"                   1 "$FIX/over_budget.log"              "$BIT_PRESENT"
check "no bitstream (didn't place) -> 1"      1 "$FIX/pass.log"                     "$BIT_ABSENT"
check "final timing FAIL -> 1"                1 "$FIX/timing_fail.log"              "$BIT_PRESENT"
check "intermediate FAIL, final PASS -> 0"    0 "$FIX/intermediate_fail_final_pass.log" "$BIT_PRESENT"

rm -f "$BIT_PRESENT"
[ "$fails" -eq 0 ] && echo "ALL PASS" || { echo "$fails FAILED"; exit 1; }
