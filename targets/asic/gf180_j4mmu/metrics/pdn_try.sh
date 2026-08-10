#!/usr/bin/env bash
# Fast PDN iteration loop: patch chip_top's config, re-run ONLY
# OpenROAD.GeneratePDN against an existing run's floorplanned ODB, and report
# the power-grid connectivity damage. ~10 minutes per hypothesis instead of the
# ~2.6 h a full chip_top route costs.
#
#   usage: metrics/pdn_try.sh '<json patch>'
#     metrics/pdn_try.sh '{"PDN_SKIPTRIM": true}'      # set a key
#     metrics/pdn_try.sh '{"PDN_EXTEND_TO": null}'     # remove a key
#
# PRECONDITION: librelane/chip_top/runs/smoke must already contain the
# pre-PDN steps (through 17-odb-addpdnobstructions). Produce them once with
#   OL_TO=OpenROAD.GeneratePDN librelane/run.sh macro=chip_top
# after which this script reuses that state -- `--from OpenROAD.GeneratePDN`
# needs a run tag that already holds the input ODB, so it must target the SAME
# tag (a fresh tag fails with "GeneratePDN: missing required input 'odb'").
#
# WHY THIS EXISTS. chip_top's OpenROAD.IRDropReport fails [PSM-0069] and is
# skipped by run.sh, so this design has never had IR-drop signoff. The same
# check also runs inside GeneratePDN (step 18), which is what makes the short
# loop possible at all. Baseline as of 2026-08-09: 20,000 dangling VDD shapes,
# 825 Metal1 followpin rails (575 at the full 1815.5 um core width + 250 at
# 8.4 um), every violation on TAP_TAPCELL/PHY_EDGE, none on an SRAM macro.
# See run.sh's OL_SKIP comment for the hypotheses already eliminated.
#
# NOTE this rewrites librelane/chip_top/config.json in place -- it is a
# bisection tool, not part of any build. Check `git diff` when you are done.
set -uo pipefail
ROOT="$(cd "$(dirname "$0")/../../../.." && pwd)"
cd "$ROOT"
[ $# -eq 1 ] || { echo "usage: $0 '<json patch>'" >&2; exit 2; }

D=targets/asic/gf180_j4mmu/librelane/chip_top
RUN="$D/runs/smoke"
OL_IMAGE="${OL_IMAGE:-ghcr.io/librelane/librelane:3.0.5}"
PDK_ROOT="${PDK_ROOT:-$HOME/.ciel}"
if [ ! -d "$RUN/17-odb-addpdnobstructions" ]; then
  echo "ERROR: $RUN has no pre-PDN state -- run this first:" >&2
  echo "  OL_TO=OpenROAD.GeneratePDN $D/../run.sh macro=chip_top" >&2
  exit 1
fi

python3 - "$1" <<'PY'
import json, sys, collections
p = 'targets/asic/gf180_j4mmu/librelane/chip_top/config.json'
d = json.load(open(p), object_pairs_hook=collections.OrderedDict)
for k, v in json.loads(sys.argv[1]).items():
    if v is None:
        d.pop(k, None)
    else:
        d[k] = v
json.dump(d, open(p, 'w'), indent=2)
open(p, 'a').write('\n')
PY
jq -s '.[0] * .[1]' "$D/../common.json" "$D/config.json" > "$D/config.merged.json"

LOG="$(mktemp)"
docker run --rm -v "$ROOT:$ROOT" -v "$PDK_ROOT:$PDK_ROOT" -w "$ROOT" \
  -e PDK_ROOT="$PDK_ROOT" "$OL_IMAGE" \
  librelane --manual-pdk --pdk-root "$PDK_ROOT" --run-tag smoke \
  --from OpenROAD.GeneratePDN --to OpenROAD.GeneratePDN \
  "$ROOT/$D/config.merged.json" > "$LOG" 2>&1
rc=$?

V="$RUN/18-openroad-generatepdn/VDD-grid-errors.rpt"
DEF="$RUN/18-openroad-generatepdn/chip_top.def"
printf 'patch=%s rc=%s\n' "$1" "$rc"
if [ ! -f "$V" ]; then
  echo "  PDN did not complete: $(grep -aoE '\[PDN-[0-9]+\][^|]*' "$LOG" | head -1)"
  echo "  full log: $LOG"
  exit 1
fi
printf '  dangling VDD shapes : %s   (baseline 20000)\n' "$(grep -ca 'violation type' "$V")"
if [ -f "$DEF" ]; then
  awk '/^SPECIALNETS/,/^END SPECIALNETS/' "$DEF" \
    | grep -oE "Metal1 [0-9]+ \+ SHAPE FOLLOWPIN \( [0-9]+ [0-9]+ \) \( [0-9]+ [0-9]+ \)" \
    | awk '{l=($11-$7)/2000; if(l<0)l=-l; n++; if(l<153.6)s++} \
           END{printf "  Metal1 rails        : %d  (%d short, %d full)   (baseline 825/250/575)\n", n, s, n-s}'
fi
echo "  full log: $LOG"
