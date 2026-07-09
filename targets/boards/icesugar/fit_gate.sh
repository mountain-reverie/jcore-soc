#!/usr/bin/env bash
# Fit + timing gate for the iCESugar UP5K build. Exits nonzero when the J1 does
# NOT fit (nextpnr did not place -> no bitstream, or ICESTORM_LC over the 5280
# budget) or a constrained clock misses its declared frequency at FINAL
# (post-route) timing. The timing check keeps the LAST verdict per clock, so an
# intermediate nextpnr estimate cannot false-positive (same rule as ulx3s).
#
# Baseline (Task 12, cpus_coremark arch — base J1 + cycle_counter +
# flash_boot_reader + ice_spi_io + W5500 SPI eth; no XIP page cache, no AIC),
# WITH the config-flash MISO pad direction fix (synth.sh step 2b: soc_gen
# infers pin_spi_miso_pin as an output since ice_spi_io's pin_* ports are
# uniformly `inout`; corrected to `in` post-regen):
# ICESTORM_LC 5093/5280 (96%, 187 LC margin), ICESTORM_RAM 17/30,
# ICESTORM_DSP 8/8 (SB_MAC16, J1 DSP multiplier), ICESTORM_SPRAM 4/4
# (SB_SPRAM256KA, spram_128k), clk_sys Fmax 14.05 MHz (PASS at 12.00 MHz
# constraint). Any regression pushing ICESTORM_LC over budget or missing
# 12 MHz timing fails this gate.
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
