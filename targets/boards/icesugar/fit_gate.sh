#!/usr/bin/env bash
# Fit + timing gate for the iCESugar UP5K build. Exits nonzero when the J1 does
# NOT fit (nextpnr did not place -> no bitstream, or ICESTORM_LC over the 5280
# budget) or a constrained clock misses its declared frequency at FINAL
# (post-route) timing. The timing check keeps the LAST verdict per clock, so an
# intermediate nextpnr estimate cannot false-positive (same rule as ulx3s).
#
# Baseline (cpus_coremark arch — base J1 + cycle_counter + flash_boot_reader +
# ice_spi_io + bidirectional uart0; no SPI eth, no XIP page cache, no AIC),
# WITH the config-flash MISO pad direction fix (synth.sh step 2b: soc_gen
# infers pin_spi_miso_pin as an output since ice_spi_io's pin_* ports are
# uniformly `inout`; corrected to `in` post-regen):
# As measured by CI (the authoritative run of this gate):
# ICESTORM_LC 5078/5280 (96%, 202 LC margin), ICESTORM_RAM 17/30,
# ICESTORM_DSP 8/8 (SB_MAC16, J1 DSP multiplier), ICESTORM_SPRAM 4/4
# (SB_SPRAM256KA, spram_128k), clk_sys Fmax 13.36 MHz (PASS at 12.00 MHz
# constraint). Any regression pushing ICESTORM_LC over budget or missing
# 12 MHz timing fails this gate.
#
# ABSOLUTE LC IS TOOLCHAIN-DEPENDENT -- compare like with like. The same tree
# measures 5105/5280 at 13.24 MHz locally against CI's 5078 at 13.36. CI and
# that local run used the SAME yosys (0.44, git sha1 80ba43d26), so the
# netlist is identical and the difference is nextpnr LUT/FF packing, not
# synthesis. Do not read a 20-30 LC move between a local run and a CI run as
# a regression; re-measure on one toolchain before concluding anything.
#
# Measured, not predicted: dropping the W5500 spi2 master and enabling the
# uart0 receiver moved LC 5093 -> 5105 (+12) and Fmax 14.05 -> 13.24 --
# both figures from the SAME local toolchain, master vs branch, so the delta
# is meaningful even though the absolutes differ from CI's. The removed SPI
# master was worth roughly 116 LC and the RX FIFO/shifter/baud logic costs
# roughly 128, so the exchange came out slightly net negative -- the opposite
# of the expectation going in. Recorded so the next person sizing a change
# against this budget starts from real numbers rather than a guess.
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
