#!/usr/bin/env bash
# Overlay the LibreLane<->ciel-PDK compatibility patches onto a freshly-enabled
# ciel GF180 PDK checkout, so the LibreLane 2.4.2 flow works reproducibly on a
# fresh machine / CI runner (not just the dev box where they were hand-applied).
#
# WHY: LibreLane 2.4.2's Tcl PDK-config loader is not compatible out of the box
# with ciel pin f6eeac7dad085ffcc829ccfd721f7b4ce39edcf7 (gf180mcuC):
#  (1) config.tcl references variable names LibreLane 2.4.2 renamed/expects
#      differently (SYNTH_DRIVING_CELL_PIN, FP_PDN_*, FP_IO_H/VLAYER,
#      FILL_CELL/DECAP_CELL);
#  (2) `dict set ::env(KEY) ...` mutations in the PDK tcl never reach LibreLane's
#      Tcl->Python bridge (TECH_LEFS/LAYERS_RC/VIAS_R arrive empty).
# The working (patched) libs.tech/librelane tcl tree is captured under this dir;
# this script copies it over the enabled PDK. Idempotent. PIN-SPECIFIC: if the
# ciel pin changes, re-capture. Cleaner long-term fix = align LibreLane+PDK-pin
# versions so no patch is needed.
set -euo pipefail
PIN="${PDK_PIN:-f6eeac7dad085ffcc829ccfd721f7b4ce39edcf7}"
CIEL_ROOT="${CIEL_ROOT:-$HOME/.ciel}"
DEST="$CIEL_ROOT/ciel/gf180mcu/versions/$PIN/gf180mcuC/libs.tech/librelane"
SRC="$(cd "$(dirname "$0")" && pwd)/gf180mcuC/libs.tech/librelane"
if [ ! -d "$DEST" ]; then
  echo "apply.sh: PDK librelane tcl dir not found at $DEST -- run 'ciel enable --pdk-family gf180mcu $PIN' first" >&2
  exit 1
fi
cp -rv "$SRC"/. "$DEST"/
echo "apply.sh: LibreLane/gf180mcuC compat overlay applied to $DEST"
