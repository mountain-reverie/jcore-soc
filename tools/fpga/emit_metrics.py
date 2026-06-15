#!/usr/bin/env python3
"""Parse jcore-soc board synthesis output into canonical metric JSON.

ECP5-only, keyed by board name. Parsers are pure (text -> dict) and unit-tested;
the CLI wires file reads and writes one canonical JSON. Best-effort: a missing
field yields no metric rather than an error. Adapted from jcore-cpu synth/metrics.py
(stripped to the ECP5 board case; adds the nextpnr PASS/FAIL timing-gate signal).
"""
import re

# nextpnr-ecp5 utilisation blocks we surface, mapped to dashboard metric labels.
NEXTPNR_BLOCKS = [
    ("TRELLIS_COMB", "LUT4"),
    ("TRELLIS_FF", "FF"),
    ("DP16KD", "DP16KD"),
    ("MULT18X18D", "MULT18X18D"),
    ("EHXPLLL", "EHXPLLL"),
    ("TRELLIS_IO", "IO"),
]


def parse_nextpnr_log(text):
    """nextpnr-ecp5 stdout -> {"util": {block: used}, "fmax": {clock: mhz}}.

    Utilisation rows look like "  TRELLIS_COMB:  5111/83640  6%"; keep `used`.
    Fmax rows: "Max frequency for clock '<name>': 78.20 MHz". Clock names are
    cleaned ($glbnet$clk:6 -> clk:6) and we keep the LAST Fmax per cleaned name
    (nextpnr prints an early estimate then the final post-route value; the last
    line is the final one).
    """
    util, fmax = {}, {}
    for line in text.splitlines():
        for blk, _label in NEXTPNR_BLOCKS:
            m = re.search(r"\b%s:\s+(\d+)/" % re.escape(blk), line)
            if m:
                util[blk] = int(m.group(1))
        m = re.search(r"Max frequency for clock '([^']+)':\s+([\d.]+)\s*MHz", line)
        if m:
            name = m.group(1).split("$")[-1]
            fmax[name] = float(m.group(2))  # last (final post-route) wins
    return {"util": util, "fmax": fmax}


def timing_failed(text):
    """True if nextpnr reported a timing violation on any constrained clock.
    nextpnr (run with --timing-allow-fail) prints "(FAIL at <freq> MHz)" when a
    domain misses its constraint and "(PASS at <freq> MHz)" when it meets it."""
    return bool(re.search(r"\(FAIL at [\d.]+\s*MHz\)", text))


def _metric(name, unit, value, direction):
    return {"name": name, "unit": unit, "value": value, "dir": direction}


def build_ecp5(parsed, board, commit):
    """Canonical doc for the ECP5 board flow. `parsed` is parse_nextpnr_log output.
    Metric names are board-prefixed so each board is its own dashboard series.

    Fmax is the LOWEST (binding) constrained clock's final value, surfaced under a
    stable "<board>/Fmax" series. We don't key on a specific clock name: nextpnr
    names the post-PLL CPU domain with an unstable yosys net id (e.g. '$glbnet$clk:6'),
    so the slowest reported domain is the robust, meaningful single figure."""
    util, fmax = parsed.get("util", {}), parsed.get("fmax", {})
    metrics_ = []
    for blk, label in NEXTPNR_BLOCKS:
        if blk in util:
            metrics_.append(_metric("%s/%s" % (board, label), "cells", util[blk], "smaller"))
    if fmax:
        metrics_.append(_metric("%s/Fmax" % board, "MHz",
                                round(min(fmax.values()), 2), "bigger"))
    return {"target": "ecp5-lfe5u-85f", "board": board, "commit": commit,
            "metrics": metrics_}


def _read(path):
    try:
        with open(path) as f:
            return f.read()
    except OSError as e:
        print("WARN: cannot read %s: %s" % (path, e))
        return ""


def main(argv=None):
    import argparse
    import json
    import os

    p = argparse.ArgumentParser(description="emit canonical ECP5 board synth metrics JSON")
    p.add_argument("--board", required=True)
    p.add_argument("--commit", required=True)
    p.add_argument("--nextpnr", required=True, help="nextpnr-ecp5 log")
    p.add_argument("--out", required=True)
    a = p.parse_args(argv)

    doc = build_ecp5(parse_nextpnr_log(_read(a.nextpnr)), a.board, a.commit)
    if not doc["metrics"]:
        print("WARN: no metrics parsed — writing empty doc")
    os.makedirs(os.path.dirname(a.out) or ".", exist_ok=True)
    with open(a.out, "w") as f:
        json.dump(doc, f, indent=2, sort_keys=False)
        f.write("\n")
    print("emit_metrics.py: wrote %d metrics to %s" % (len(doc["metrics"]), a.out))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
