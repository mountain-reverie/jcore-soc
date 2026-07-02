#!/usr/bin/env bash
# Unit test for check_variant.sh: feed a fixture cpus_config.vhd + a VARIANT and
# assert the guard's exit code.
set -uo pipefail
HERE="$(cd "$(dirname "$0")" && pwd)"
CHK="$HERE/../check_variant.sh"
FIX="$HERE/fixtures"
fails=0

check() {  # <expected-rc> <variant> <cfg-fixture> <desc>
  "$CHK" "$3" "$2" >/dev/null 2>&1; local rc=$?
  if [ "$rc" -ne "$1" ]; then echo "FAIL: $4 (rc=$rc want $1)"; fails=$((fails+1));
  else echo "ok: $4"; fi
}

check 0 j2-direct "$FIX/cpus_config_direct.vhd" "direct cfg + j2-direct -> 0"
check 1 j4-rom    "$FIX/cpus_config_direct.vhd" "direct cfg + j4-rom -> 1 (mismatch)"
check 0 j4-rom    "$FIX/cpus_config_j4rom.vhd"  "j4rom cfg + j4-rom -> 0"
check 1 bogus     "$FIX/cpus_config_direct.vhd" "unknown variant -> 1"

[ "$fails" -eq 0 ] && echo "ALL PASS" || { echo "$fails FAILED"; exit 1; }
