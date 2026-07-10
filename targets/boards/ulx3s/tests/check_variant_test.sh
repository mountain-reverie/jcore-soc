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
# dual-core variants share their decode with the single-core counterpart, so the
# check anchors on the same cpu_synth config (j2-dual->cpu_synth_direct,
# j4-dual->cpu_synth_j4_rom; j4-dual's asymmetric core0 _ebr line is not matched).
check 0 j2-dual "$FIX/cpus_config_direct.vhd" "direct cfg + j2-dual -> 0"
check 0 j4-dual "$FIX/cpus_config_j4rom.vhd"  "j4rom cfg + j4-dual -> 0"
check 1 j4-dual "$FIX/cpus_config_direct.vhd" "direct cfg + j4-dual -> 1 (mismatch)"

[ "$fails" -eq 0 ] && echo "ALL PASS" || { echo "$fails FAILED"; exit 1; }
