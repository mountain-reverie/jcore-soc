#!/usr/bin/env python3
"""Parse yosys `stat -liberty` output into canonical GF180 metric JSON.

Two things live here:

1. `parse_yosys_stat(text)` -- a PURE text -> dict parser (best-effort:
   missing field -> no key), mirroring tools/fpga/emit_metrics.py. Extracts
   cell count, chip area, and (when present) the sequential-elements
   fraction line yosys's `stat -liberty` prints (`of which used for
   sequential elements: <area> (<pct>%)`).

2. A CLI that takes MULTIPLE per-macro stat files + a commit and writes ONE
   canonical doc consumable by tools/fpga/to_gha_bench.py:
     {"target","board","commit","metrics":[{"name","unit","value","dir"},...]}
   plus a non-charted `macros` field per macro so the memory-as-flops caveat
   (final-review I1/T3) travels with the published JSON:
     - the flat whole-SoC `top` macro is DROPPED from `metrics` entirely (it
       is not a silicon-area proxy: memory-inflated AND flattened
       hierarchy). It still appears (without cells/area metrics charted) if
       passed in, for traceability.
     - macros marked "inflated" in macros.list (their numbers include
       GHDL-elaboration-inferred RAM-as-flops -- see synth_gf180.sh/
       macros.list header comments) get an "incl-inferred-RAM" suffix baked
       into their published series name, so a memory-inflated number can
       never be mistaken for a clean logic-area one at a glance.
     - each macro's `seq_pct` (from parse_yosys_stat) is carried verbatim in
       doc["macros"][<macro>]["seq_pct"].
   The macro -> published-name / inflated-or-not mapping is read from
   macros.list's 4th column, not hardcoded here (final-review nit).

Usage:
   emit_gf180_metrics.py j4_core=build/j4_core.stat.txt \\
       cache_ctrl.icache=build/cache_ctrl.icache.stat.txt ... \\
       --commit <sha> --out metrics-gf180.json
"""
import argparse
import json
import re
import sys
from pathlib import Path


def parse_yosys_stat(text):
    out = {}
    m = re.search(r"Number of cells:\s+(\d+)", text)
    if m:
        out["cells"] = int(m.group(1))
    m = re.search(r"Chip area for module.*?:\s+([\d.]+)", text)
    if m:
        out["area_um2"] = float(m.group(1))
    m = re.search(
        r"of which used for sequential elements:\s+([\d.]+)\s+\(([\d.]+)%\)", text
    )
    if m:
        out["seq_area_um2"] = float(m.group(1))
        out["seq_pct"] = float(m.group(2))
    return out


def parse_macros_list(path):
    """Return {macro: inflated(bool)} from macros.list's 4th column."""
    inflated = {}
    for line in Path(path).read_text().splitlines():
        line = line.strip()
        if not line or line.startswith("#"):
            continue
        fields = line.split()
        macro = fields[0]
        flag = fields[3] if len(fields) > 3 else "-"
        inflated[macro] = flag == "inflated"
    return inflated


def build_canonical_doc(stat_files, commit, macros_list, target, board):
    """stat_files: {macro: path_to_stat_txt}. Returns the canonical doc dict."""
    inflated_map = parse_macros_list(macros_list)
    metrics = []
    macro_details = {}
    for macro, stat_path in sorted(stat_files.items()):
        text = Path(stat_path).read_text()
        m = parse_yosys_stat(text)
        is_inflated = inflated_map.get(macro, False)
        macro_details[macro] = dict(m)
        macro_details[macro]["inflated"] = is_inflated

        if macro == "top":
            # Flat whole-SoC total: not a silicon-area proxy (memory-as-flops
            # AND flattened hierarchy). Deliberately not charted -- see
            # macros.list/synth_gf180.sh header comments.
            continue

        label = f"{macro} incl-inferred-RAM" if is_inflated else macro
        name_prefix = f"gf180_j4mmu [{label}]"
        if "cells" in m:
            metrics.append(
                {"name": f"{name_prefix}/cells", "unit": "cells",
                 "value": m["cells"], "dir": "smaller"}
            )
        if "area_um2" in m:
            metrics.append(
                {"name": f"{name_prefix}/area_um2", "unit": "um2",
                 "value": m["area_um2"], "dir": "smaller"}
            )
    return {
        "target": target,
        "board": board,
        "commit": commit,
        "metrics": metrics,
        "macros": macro_details,
    }


def main(argv=None):
    ap = argparse.ArgumentParser()
    ap.add_argument("stats", nargs="+", metavar="MACRO=STAT_FILE",
                     help="macro=stat.txt pairs, e.g. j4_core=build/j4_core.stat.txt")
    ap.add_argument("--commit", required=True)
    ap.add_argument("--out", required=True)
    ap.add_argument("--macros-list",
                     default="targets/asic/gf180_j4mmu/metrics/macros.list")
    ap.add_argument("--target", default="gf180mcu-mcu7t5v0")
    ap.add_argument("--board", default="gf180_j4mmu")
    a = ap.parse_args(argv)

    stat_files = {}
    for pair in a.stats:
        macro, sep, path = pair.partition("=")
        if not sep:
            ap.error(f"expected MACRO=STAT_FILE, got: {pair!r}")
        stat_files[macro] = path

    doc = build_canonical_doc(stat_files, a.commit, a.macros_list, a.target, a.board)
    with open(a.out, "w") as f:
        json.dump(doc, f, indent=2)
    published = len(stat_files) - (1 if "top" in stat_files else 0)
    print(f"emit_gf180_metrics.py: wrote {len(doc['metrics'])} metrics "
          f"for {published} published macros to {a.out}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
