#!/usr/bin/env bash
# filelist_core_drift.sh -- every core CPU source must reach the ghdl filelists.
#
# WHY: targets/*/filelist.sh are HAND-WRITTEN. They must be, because ghdl needs
# a topological analyse order and components/cpu/build_core.mk is an unordered
# make set (it lists core/cpu.vhd third, before its own dependencies). So the
# ORDER cannot be generated from it -- but MEMBERSHIP can be checked, and that
# is the failure mode that actually bites:
#
#   core/tlb_walk.vhd was added to build_core.mk when the hardware TSB walker
#   landed. All three filelists missed it, and every board/ASIC synth failed
#   with `unit "tlb_walk" not found in library "work"` -- a submodule bump
#   surfacing as an elaboration error with no obvious connection to the change.
#
# A file may be legitimately absent (variant-specific, or an alternate decode
# table this target does not build). Those go in EXCLUDE below WITH A REASON.
# The list rots loudly: adding a core source forces you through here.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"; cd "$ROOT"

CORE_MK="components/cpu/build_core.mk"
[ -f "$CORE_MK" ] || { echo "ERROR: $CORE_MK missing (submodule not checked out?)" >&2; exit 1; }

# Absent by design. Keep the reason attached.
EXCLUDE_RE='^(core/cpu_config_common\.vhd|core/register_file_ebr\.vhd|core/shifter_seq\.vhd|decode/decode_table_(direct|rom|simple)_config\.vhd)$'
#   cpu_config_common / register_file_ebr / shifter_seq : variant-specific,
#     pulled in per-target via CPU_EXTRA_FILES rather than the base list.
#   decode_table_{direct,rom,simple}_config : each target builds exactly ONE
#     decode table (gf180/ulx3s direct, icesugar rom) and which one is named
#     by the soc_gen-GENERATED cpu_synth_files.list -- i.e. that selection is
#     already machine-maintained and outside this check's remit.

# Plain paths only -- skip make variables and $(patsubst ...) constructs.
CORE="$(grep -oE '^\$\(VHDLS\) \+= [^$[:space:]]+' "$CORE_MK" | awk '{print $3}' \
        | grep -vE "$EXCLUDE_RE" | sort -u)"

rc=0
for fl in targets/asic/gf180_j4mmu/filelist.sh \
          targets/boards/ulx3s/filelist.sh \
          targets/boards/icesugar/filelist.sh; do
  # A file counts as present if it is named in filelist.sh itself OR in the
  # soc_gen-generated cpu_synth_files.list that filelist.sh splices in --
  # the variant-specific decode/config sources arrive by that route.
  gen_list="$(dirname "$fl")/cpu_synth_files.list"
  haystack="$(cat "$fl" ${gen_list:+$([ -f "$gen_list" ] && echo "$gen_list")} 2>/dev/null)"
  missing=""
  while read -r f; do
    printf '%s' "$haystack" | grep -qF -- "$f" || missing="$missing $f"
  done <<< "$CORE"
  if [ -n "$missing" ]; then
    echo "ERROR: $fl is missing core sources from $CORE_MK:" >&2
    for m in $missing; do echo "    $m" >&2; done
    rc=1
  fi
done

if [ "$rc" != 0 ]; then
  echo "" >&2
  echo "Add them in dependency order (ghdl analyses in sequence), or add an" >&2
  echo "entry to EXCLUDE in this script with the reason it does not apply." >&2
  exit 1
fi
echo "OK: all core sources in $CORE_MK appear in every filelist.sh"
