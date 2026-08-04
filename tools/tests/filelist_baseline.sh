#!/bin/sh
# Dump each board's resolved VHDL file list so a refactor can be proven
# list-neutral. Usage: tools/tests/filelist_baseline.sh <outdir>
#
# Uses silent make (-s) to avoid console noise contaminating output, validates
# that output contains actual .vhd paths, and fails loudly on invalid input.
# Exits non-zero if any board fails, ensuring stale files don't mask failures.
set -e
out="$1"; [ -n "$out" ] || { echo "usage: $0 <outdir>" >&2; exit 1; }
mkdir -p "$out"

failures=""

for d in targets/boards/*/Makefile; do
  b=$(basename "$(dirname "$d")")

  # Remove stale files from previous runs before attempting capture. This ensures
  # that a failed board leaves no file behind (a stale file would mask the failure).
  rm -f "$out/$b.vhdl_files" "$out/$b.cpu_synth_files"

  # Run make in silent mode to avoid console noise contaminating output.
  if ! make -s "$b" TARGET=print-vhdl-files >"$out/$b.vhdl_files.raw" 2>/dev/null; then
    echo "SKIP $b (make failed)" >&2
    failures="$failures $b"
    continue
  fi

  # Normalize: split space-delimited, remove empty lines, sort alphabetically.
  tr ' ' '\n' <"$out/$b.vhdl_files.raw" | grep -v '^$' | sort >"$out/$b.vhdl_files.tmp"

  # Validate: must have at least one line ending in .vhd or .vhh (VHDL files).
  vhdl_count=$(grep -cE '\.(vhd|vhh)$' "$out/$b.vhdl_files.tmp" || true)
  if [ "$vhdl_count" -lt 1 ]; then
    echo "SKIP $b (no valid .vhd or .vhh files in output)" >&2
    failures="$failures $b"
    rm -f "$out/$b.vhdl_files.tmp" "$out/$b.vhdl_files.raw"
    continue
  fi

  # Validate: all lines must end in .vhd or .vhh (no shell/make output mixed in).
  total_count=$(wc -l <"$out/$b.vhdl_files.tmp")
  if [ "$vhdl_count" -ne "$total_count" ]; then
    echo "SKIP $b (output contains non-VHDL lines)" >&2
    failures="$failures $b"
    rm -f "$out/$b.vhdl_files.tmp" "$out/$b.vhdl_files.raw"
    continue
  fi

  # Success: move to final output.
  mv "$out/$b.vhdl_files.tmp" "$out/$b.vhdl_files"
  rm -f "$out/$b.vhdl_files.raw"

  # Also capture cpu_synth_files if it exists.
  l="targets/boards/$b/generated/cpu_synth_files.list"
  [ -f "$l" ] && sort "$l" >"$out/$b.cpu_synth_files"
done

# Check if any boards failed and exit non-zero with a clear message.
# Process all boards first (don't bail on first failure) for a complete picture.
if [ -n "$failures" ]; then
  echo "ERROR: baseline capture failed for boards:$failures" >&2
  exit 1
fi
