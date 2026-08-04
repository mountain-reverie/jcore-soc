#!/bin/sh
# Dump each board's resolved VHDL file list so a refactor can be proven
# list-neutral. Usage: tools/tests/filelist_baseline.sh <outdir>
set -e
out="$1"; [ -n "$out" ] || { echo "usage: $0 <outdir>" >&2; exit 1; }
mkdir -p "$out"
for d in targets/boards/*/Makefile; do
  b=$(basename "$(dirname "$d")")
  make "$b" TARGET=print-vhdl-files >"$out/$b.vhdl_files.raw" 2>/dev/null || {
    echo "SKIP $b (no print-vhdl-files target)" >&2; continue; }
  tr ' ' '\n' <"$out/$b.vhdl_files.raw" | grep -v '^$' | sort >"$out/$b.vhdl_files"
  rm -f "$out/$b.vhdl_files.raw"
  l="targets/boards/$b/generated/cpu_synth_files.list"
  [ -f "$l" ] && sort "$l" >"$out/$b.cpu_synth_files"
done
