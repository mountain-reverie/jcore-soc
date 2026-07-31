#!/usr/bin/env bash
# Finish the flat pad_ring route with direct OpenROAD (see route.tcl for WHY:
# LibreLane's grt step errors on GRT-0118; global_route -allow_congestion
# pushes past it). run.sh macro=pad_ring must have hardened through CTS first.
#
# OpenROAD `read_db <cts.odb>` restores the WHOLE design (tech + std-cell +
# macro LEFs, placement, CTS, nets) from the serialized odb, so no read_lef
# preamble is needed here -- route.tcl just operates on the loaded block.
#
# Outputs (into pad_ring/runs/): routed_flat.def, padring_metrics.json (die
# area + DRC count, consumed by ../metrics/gf180_die.sh), flatpr_drc.rpt.
set -uo pipefail
HERE="$(cd "$(dirname "$0")" && pwd)"
ROOT="$(cd "$HERE/../../../../.." && pwd)"
OL_IMAGE="${OL_IMAGE:-ghcr.io/librelane/librelane:2.4.2}"
RUNS="$HERE/runs"

# newest CTS odb from the run.sh macro=pad_ring harden
ODB="$(find "$RUNS" -name '*.odb' -path '*cts*' 2>/dev/null | sort | tail -1)"
[ -z "$ODB" ] && ODB="$(find "$RUNS" -name '*.odb' 2>/dev/null | sort | tail -1)"
if [ -z "$ODB" ]; then
  echo "ERROR: no post-CTS .odb under $RUNS -- run ./run.sh macro=pad_ring first." >&2
  exit 1
fi
echo "finish_route.sh: reading $ODB" >&2

# path of the odb / outputs *inside* the container (repo mounted at /work)
rel() { echo "/work/${1#$ROOT/}"; }
PDK_ROOT="${PDK_ROOT:-$(find "$HOME/.ciel" -maxdepth 4 -type d -name 'gf180mcuC' 2>/dev/null | head -1)}"

# driver tcl written into the mounted runs dir (process substitution can't
# cross the docker boundary): read the restored design, then source route.tcl.
DRIVER="$RUNS/_finish_route.tcl"
{ echo "read_db $(rel "$ODB")"; echo "source $(rel "$HERE/route.tcl")"; } > "$DRIVER"

docker run --rm \
  -v "$ROOT":/work -v "$PDK_ROOT":/pdk \
  -e PADRING_DEF="$(rel "$RUNS/routed_flat.def")" \
  -e PADRING_DRC="$(rel "$RUNS/flatpr_drc.rpt")" \
  -e PADRING_JSON="$(rel "$RUNS/padring_metrics.json")" \
  "$OL_IMAGE" \
  openroad -exit -threads max "$(rel "$DRIVER")" \
  || { echo "WARN: openroad route.tcl exited non-zero (check DRC)" >&2; }

echo "finish_route.sh: done -> $RUNS/routed_flat.def" >&2
[ -f "$RUNS/padring_metrics.json" ] && cat "$RUNS/padring_metrics.json"
