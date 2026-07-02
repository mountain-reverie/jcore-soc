#!/usr/bin/env bash
# Verify the generated ULX3S cpus_config.vhd was produced for $VARIANT, so a local
# `VARIANT=x ./synth.sh` cannot synthesize one design and label its metrics as
# another. The variant->config mapping mirrors tools/socgen/elaborate/cpumap.go
# (the source of truth); keep them in sync when adding ULX3S variants.
# Usage: check_variant.sh <cpus_config.vhd> <variant>
set -uo pipefail
CFG="${1:?usage: check_variant.sh <cpus_config.vhd> <variant>}"
VARIANT="${2:?usage: check_variant.sh <cpus_config.vhd> <variant>}"

case "$VARIANT" in
  j2-direct) expect="cpu_synth_direct" ;;
  j4-rom)    expect="cpu_synth_j4_rom" ;;
  *) echo "check_variant: unknown VARIANT '$VARIANT' (known: j2-direct, j4-rom)" >&2; exit 1 ;;
esac

if grep -qE "use configuration work\.${expect}([ ;]|$)" "$CFG"; then
  echo "ULX3S: cpus_config.vhd matches VARIANT=$VARIANT (binds work.$expect)"
  exit 0
fi
echo "ULX3S: cpus_config.vhd is NOT generated for VARIANT=$VARIANT (expected work.$expect)." >&2
echo "       Run: make ulx3s TARGET=soc_gen VARIANT=$VARIANT" >&2
exit 1
