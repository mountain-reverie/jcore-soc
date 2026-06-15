#!/usr/bin/env python3
"""Merge canonical board-metric JSONs into github-action-benchmark 'custom' inputs.

Splits metrics by direction into two arrays:
  smaller -> customSmallerIsBetter   bigger -> customBiggerIsBetter
Each entry: {"name","unit","value","extra"}. `name` is prefixed with the target
("ecp5-lfe5u-85f · ulx3s/LUT4"); the board name is already in the metric name, so
each board is a distinct series automatically. `extra` carries the board.
Output is sorted by name for deterministic diffs. Adapted from jcore-cpu
synth/to_gha_bench.py (board-keyed; no J1/J2/J4 variant machinery).
"""
import json


def convert(canon_paths):
    size, speed = [], []
    for path in canon_paths:
        with open(path) as f:
            doc = json.load(f)
        try:
            target, board = doc["target"], doc.get("board", "")
            metrics = doc["metrics"]
        except KeyError as e:
            raise ValueError("malformed canonical doc in %s: missing key %s" % (path, e)) from e
        for m in metrics:
            entry = {
                "name": "%s · %s" % (target, m["name"]),
                "unit": m["unit"],
                "value": m["value"],
                "extra": board,
            }
            (size if m["dir"] == "smaller" else speed).append(entry)
    size.sort(key=lambda e: e["name"])
    speed.sort(key=lambda e: e["name"])
    return size, speed


def main(argv=None):
    import argparse
    p = argparse.ArgumentParser()
    p.add_argument("inputs", nargs="+", help="canonical metric JSON files")
    p.add_argument("--size-out", required=True)
    p.add_argument("--speed-out", required=True)
    a = p.parse_args(argv)
    size, speed = convert(a.inputs)
    for path, data in ((a.size_out, size), (a.speed_out, speed)):
        with open(path, "w") as f:
            json.dump(data, f, indent=2)
            f.write("\n")
    print("to_gha_bench.py: %d size, %d speed metrics" % (len(size), len(speed)))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
