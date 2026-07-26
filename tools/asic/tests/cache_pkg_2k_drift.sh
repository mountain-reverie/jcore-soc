#!/usr/bin/env bash
# cache_pkg_2k_drift.sh -- Task 4 drift guard.
#
# targets/asic/gf180_j4mmu/cache_pkg_2k.vhd is a target-local COPY of the
# submodule components/cpu/cache/cache_pkg.vhd (same package name
# `cache_pack`) with only CACHE_INDEX_BITS overridden (8 -> 6, 8 KB -> 2 KB).
# The submodule itself must never be edited for this (see cache_pkg_2k.vhd's
# header / task-4 brief). This guard fails loudly the moment the two files
# drift apart in any OTHER way -- e.g. an upstream cache_pkg.vhd change that
# this target-local copy silently misses -- by asserting the unified diff
# between them touches exactly one line, and that line is the
# CACHE_INDEX_BITS constant.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../../.." && pwd)"
cd "$ROOT"

SUBMODULE="components/cpu/cache/cache_pkg.vhd"
OVERRIDE="targets/asic/gf180_j4mmu/cache_pkg_2k.vhd"

for f in "$SUBMODULE" "$OVERRIDE"; do
  if [ ! -f "$f" ]; then
    echo "ERROR: $f missing" >&2
    exit 1
  fi
done

# Unified diff with zero context: one line changed -> exactly one '-' and
# one '+' content line (plus the two '@@'-less hunk/file headers are
# suppressed by -U0's own hunk header, which we also count and expect
# exactly one of).
DIFF="$(diff -U0 "$SUBMODULE" "$OVERRIDE" || true)"
HUNKS="$(printf '%s\n' "$DIFF" | grep -c '^@@' || true)"
CHANGED_MINUS="$(printf '%s\n' "$DIFF" | grep '^-' | grep -v '^---' || true)"
CHANGED_PLUS="$(printf '%s\n' "$DIFF" | grep '^+' | grep -v '^+++' || true)"

if [ -z "$DIFF" ]; then
  echo "ERROR: $OVERRIDE is byte-identical to $SUBMODULE (expected CACHE_INDEX_BITS override)" >&2
  exit 1
fi

if [ "$HUNKS" -ne 1 ]; then
  echo "ERROR: $OVERRIDE drifted from $SUBMODULE in more than one place ($HUNKS hunks):" >&2
  printf '%s\n' "$DIFF" >&2
  exit 1
fi

if [ "$(printf '%s\n' "$CHANGED_MINUS" | wc -l)" -ne 1 ] || [ "$(printf '%s\n' "$CHANGED_PLUS" | wc -l)" -ne 1 ]; then
  echo "ERROR: $OVERRIDE's single hunk is not a single-line change:" >&2
  printf '%s\n' "$DIFF" >&2
  exit 1
fi

if ! printf '%s\n' "$CHANGED_MINUS" | grep -q 'CACHE_INDEX_BITS'; then
  echo "ERROR: the one changed line is not the CACHE_INDEX_BITS constant:" >&2
  printf '%s\n' "$DIFF" >&2
  exit 1
fi

if ! printf '%s\n' "$CHANGED_PLUS" | grep -q 'CACHE_INDEX_BITS.*:= *6'; then
  echo "ERROR: $OVERRIDE's CACHE_INDEX_BITS override is not := 6 (2 KB):" >&2
  printf '%s\n' "$DIFF" >&2
  exit 1
fi

echo "OK: $OVERRIDE differs from $SUBMODULE only in CACHE_INDEX_BITS (8 -> 6, 8KB -> 2KB)"
