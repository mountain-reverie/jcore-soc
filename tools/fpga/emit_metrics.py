#!/usr/bin/env python3
"""Parse jcore-soc board synthesis output into canonical metric JSON.

Supports ECP5 and iCE40 flows. Parsers are pure (text -> dict) and unit-tested;
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

# nextpnr-ice40 utilisation blocks we surface, mapped to dashboard metric labels.
NEXTPNR_ICE40_BLOCKS = [
    ("ICESTORM_LC", "LC"),
    ("ICESTORM_RAM", "RAM"),
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


def parse_nextpnr_ice40_log(text):
    """nextpnr-ice40 stdout -> {"util": {block: used}, "fmax": {clock: mhz}}.
    Utilisation rows: "  ICESTORM_LC:  5443/ 5280  103%" (keep `used`). Fmax rows
    reuse the ECP5 format; clock names are cleaned and the LOWEST per name kept
    (the binding post-route constraint)."""
    util, fmax = {}, {}
    for line in text.splitlines():
        for blk, _label in NEXTPNR_ICE40_BLOCKS:
            m = re.search(r"\b%s:\s+(\d+)/" % re.escape(blk), line)
            if m:
                util[blk] = int(m.group(1))
        m = re.search(r"Max frequency for clock '([^']+)':\s+([\d.]+)\s*MHz", line)
        if m:
            name = m.group(1).split("$")[-1]
            val = float(m.group(2))
            if name not in fmax or val < fmax[name]:
                fmax[name] = val
    return {"util": util, "fmax": fmax}


def parse_yosys_stat(text):
    """yosys `stat` (post synth_ice40) -> {cell: count}.

    Real ``synth_ice40`` stat emits SB_* cell names, not ICESTORM_* names.
    We map the two authoritative SB cells to their canonical ICESTORM_* keys so
    downstream code (build_ice40) can treat yosys-stat as a (secondary) source of
    LC/RAM counts.  ICESTORM_* rows are also accepted for hypothetical inputs that
    already carry them.
    """
    # SB_* cells emitted by real yosys synth_ice40 -> canonical key
    SB_MAP = {
        "SB_LUT4": "ICESTORM_LC",
        "SB_RAM40_4K": "ICESTORM_RAM",
        "SB_SPRAM256KA": "ICESTORM_SPRAM",
    }
    out = {}
    for line in text.splitlines():
        # Accept canonical ICESTORM_* rows if present
        m = re.search(r"\b(ICESTORM_LC|ICESTORM_RAM|ICESTORM_SPRAM):\s+(\d+)", line)
        if m:
            out[m.group(1)] = int(m.group(2))
            continue
        # Map real SB_* rows to canonical keys
        for sb, canonical in SB_MAP.items():
            m = re.search(r"\b%s\s+(\d+)" % re.escape(sb), line)
            if m:
                out[canonical] = int(m.group(1))
    return out


def timing_failed(text):
    """True if nextpnr reported a timing violation on any constrained clock.
    nextpnr (run with --timing-allow-fail) prints "(FAIL at <freq> MHz)" when a
    domain misses its constraint and "(PASS at <freq> MHz)" when it meets it."""
    return bool(re.search(r"\(FAIL at [\d.]+\s*MHz\)", text))


def _metric(name, unit, value, direction):
    return {"name": name, "unit": unit, "value": value, "dir": direction}


def _vname(board, variant, label):
    """'<board> [<variant>]/<label>' (or '<board>/<label>' when variant is falsy).
    The space-bracket suffix is what the dashboard regex keys variants on."""
    head = "%s [%s]" % (board, variant) if variant else board
    return "%s/%s" % (head, label)


def build_ecp5(parsed, board, commit, variant=None):
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
            metrics_.append(_metric(_vname(board, variant, label), "cells", util[blk], "smaller"))
    if fmax:
        metrics_.append(_metric(_vname(board, variant, "Fmax"), "MHz",
                                round(min(fmax.values()), 2), "bigger"))
    return {"target": "ecp5-lfe5u-85f", "board": board, "commit": commit,
            "metrics": metrics_}


def build_ice40(stat, parsed_pnr, board, commit, variant=None):
    """Canonical doc for the iCE40 flow. LC/RAM come from the yosys `stat`
    (always present); Fmax from nextpnr only when it routed (parsed_pnr.fmax).
    yosys synth_ice40 uses SB_* cell names, while nextpnr reports ICESTORM_*
    names in its utilisation summary (printed even on placement failure).
    Fall back to nextpnr util so LC counts are available regardless of route."""
    fmax = (parsed_pnr or {}).get("fmax", {})
    pnr_util = (parsed_pnr or {}).get("util", {})
    # nextpnr post-pack util takes precedence over yosys pre-pack stat for LC/RAM:
    # nextpnr prints ICESTORM_* utilisation even on placement failure, and the
    # post-pack count is the authoritative "fit" figure (yosys stat is pre-pack
    # and may differ from what nextpnr actually uses).
    combined = {**stat, **pnr_util}
    metrics_ = []
    for cell, label in NEXTPNR_ICE40_BLOCKS:
        if cell in combined:
            metrics_.append(_metric(_vname(board, variant, label), "cells", combined[cell], "smaller"))
    if fmax:
        metrics_.append(_metric(_vname(board, variant, "Fmax"), "MHz",
                                round(min(fmax.values()), 2), "bigger"))
    return {"target": "ice40-up5k", "board": board, "commit": commit, "metrics": metrics_}


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

    p = argparse.ArgumentParser(description="emit canonical board synth metrics JSON")
    p.add_argument("--board", required=True)
    p.add_argument("--commit", required=True)
    p.add_argument("--flow", choices=["ecp5", "ice40"], default="ecp5")
    p.add_argument("--variant", default=None, help="variant tag for metric names")
    p.add_argument("--nextpnr", help="nextpnr log (required for ecp5, optional for ice40)")
    p.add_argument("--yosys-stat", help="yosys stat output (required for ice40)")
    p.add_argument("--out", required=True)
    a = p.parse_args(argv)

    if a.flow == "ecp5":
        if not a.nextpnr:
            p.error("--nextpnr is required for ecp5 flow")
        doc = build_ecp5(parse_nextpnr_log(_read(a.nextpnr)), a.board, a.commit, a.variant)
    elif a.flow == "ice40":
        if not a.yosys_stat:
            p.error("--yosys-stat is required for ice40 flow")
        stat = parse_yosys_stat(_read(a.yosys_stat))
        parsed_pnr = parse_nextpnr_ice40_log(_read(a.nextpnr)) if a.nextpnr else {"util": {}, "fmax": {}}
        doc = build_ice40(stat, parsed_pnr, a.board, a.commit, a.variant)
    else:
        p.error("unknown flow: %s" % a.flow)

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
