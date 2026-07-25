#!/usr/bin/env python3
"""Parse LibreLane place & route metrics into canonical GF180 die-area JSON.

Companion to emit_gf180_metrics.py (which handles yosys synth-only stats).
This one handles the LibreLane P&R side of the GF180 sub-project: routed
LEF/GDS area figures for the 8 hardened macros (Tasks 2-6) and the
integrated `top` (Task 7's 21.72mm2 padded die).

LibreLane emits `design__die__area` / `design__core__area` /
`design__instance__area` (all um2, as floats or ints) in two places per
step: `runs/<tag>/<step>/or_metrics_out.json` (one per pipeline step) and
`runs/<tag>/final/metrics.json` (the last successful step's numbers,
written once the run reaches its stop point / completes). Both have the
identical flat key shape, so `parse_or_metrics()` reads either.

The `top` run is known to error out in its GDS-merge finishing step (see
targets/asic/gf180_j4mmu/librelane/run.sh) -- it never reaches `final/`.
gf180_die.sh handles that by pointing this script at the last successful
step's `or_metrics_out.json` (the routing/RCX step) instead of `final/
metrics.json` for `top`; parse_or_metrics() doesn't care which file it's
given, so no top-specific code lives here.

Output shape matches emit_gf180_metrics.py's canonical doc so both feed
tools/fpga/to_gha_bench.py unchanged:
    {"target","board","commit","metrics":[{"name","unit","value","dir"},...]}

Series emitted (unit mm2, dir=smaller-is-better):
  gf180-die-area-mm2            -- top, routed die area
  gf180-core-area-mm2           -- top, routed core area
  gf180-placed-silicon-mm2      -- top, placed-instance area (silicon proxy)
  gf180-die-area-mm2 [<macro>]  -- per-macro routed die area, one per macro
  kianv-die-20.1mm2             -- ALWAYS emitted: KianV's reference die
                                    area (20.1mm2, pinned constant, not
                                    parsed from any run) so the dashboard
                                    always has a limit line to compare
                                    against, even on a partial P&R.

Usage:
  emit_die_metrics.py --commit <sha> --out metrics-die.json \\
      [--top librelane/top/runs/smoke/48-openroad-rcx/or_metrics_out.json] \\
      [--macro j4_core=librelane/j4_core/runs/smoke/final/metrics.json ...]

Both --top and any number of --macro are optional -- if a macro or the top
run didn't complete, just omit it; whatever IS available still gets
published (gf180_die.sh's whole point: a partial P&R still updates the
graph).
"""
import argparse
import json
import sys
from pathlib import Path

KIANV_DIE_MM2 = 20.1


def parse_or_metrics(path):
    """Read a LibreLane or_metrics_out.json / final/metrics.json.

    Returns {"die_um2","core_um2","instance_um2"} -- missing keys omitted.
    """
    doc = json.loads(Path(path).read_text())
    out = {}
    key_map = {
        "design__die__area": "die_um2",
        "design__core__area": "core_um2",
        "design__instance__area": "instance_um2",
    }
    for src_key, dst_key in key_map.items():
        if src_key in doc and doc[src_key] is not None:
            out[dst_key] = float(doc[src_key])
    return out


def _mm2(um2):
    return round(um2 / 1e6, 6)


def build_die_doc(top_metrics, macro_metrics, commit,
                   target="gf180mcu-mcu7t5v0", board="gf180_j4mmu"):
    """top_metrics: path or None. macro_metrics: {macro: path}."""
    metrics = []

    if top_metrics is not None:
        m = parse_or_metrics(top_metrics)
        if "die_um2" in m:
            metrics.append({"name": "gf180-die-area-mm2", "unit": "mm2",
                             "value": _mm2(m["die_um2"]), "dir": "smaller"})
        if "core_um2" in m:
            metrics.append({"name": "gf180-core-area-mm2", "unit": "mm2",
                             "value": _mm2(m["core_um2"]), "dir": "smaller"})
        if "instance_um2" in m:
            metrics.append({"name": "gf180-placed-silicon-mm2", "unit": "mm2",
                             "value": _mm2(m["instance_um2"]), "dir": "smaller"})

    for macro, path in sorted(macro_metrics.items()):
        m = parse_or_metrics(path)
        if "die_um2" in m:
            metrics.append({
                "name": f"gf180-die-area-mm2 [{macro}]", "unit": "mm2",
                "value": _mm2(m["die_um2"]), "dir": "smaller",
            })

    # Pinned reference constant: always emitted, independent of whether any
    # P&R data was available, so the dashboard always draws the limit line.
    metrics.append({
        "name": "kianv-die-20.1mm2", "unit": "mm2",
        "value": KIANV_DIE_MM2, "dir": "smaller",
    })

    return {
        "target": target,
        "board": board,
        "commit": commit,
        "metrics": metrics,
    }


def main(argv=None):
    ap = argparse.ArgumentParser()
    ap.add_argument("--top", default=None,
                     help="path to top's or_metrics_out.json / metrics.json")
    ap.add_argument("--macro", action="append", default=[],
                     metavar="MACRO=METRICS_JSON",
                     help="repeatable: per-macro metrics.json, e.g. "
                          "j4_core=librelane/j4_core/runs/smoke/final/metrics.json")
    ap.add_argument("--commit", required=True)
    ap.add_argument("--out", required=True)
    ap.add_argument("--target", default="gf180mcu-mcu7t5v0")
    ap.add_argument("--board", default="gf180_j4mmu")
    a = ap.parse_args(argv)

    macro_metrics = {}
    for pair in a.macro:
        macro, sep, path = pair.partition("=")
        if not sep:
            ap.error(f"expected MACRO=METRICS_JSON, got: {pair!r}")
        macro_metrics[macro] = path

    doc = build_die_doc(a.top, macro_metrics, a.commit, a.target, a.board)
    Path(a.out).parent.mkdir(parents=True, exist_ok=True)
    with open(a.out, "w") as f:
        json.dump(doc, f, indent=2)
        f.write("\n")
    print(f"emit_die_metrics.py: wrote {len(doc['metrics'])} metrics "
          f"({len(macro_metrics)} macros, top={'yes' if a.top else 'no'}) to {a.out}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
