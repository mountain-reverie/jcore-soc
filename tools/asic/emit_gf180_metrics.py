#!/usr/bin/env python3
"""Parse yosys `stat -liberty` output into canonical GF180 metric JSON.
Pure text->dict parser (best-effort: missing field -> no key), mirroring
tools/fpga/emit_metrics.py."""
import argparse, json, re, sys

def parse_yosys_stat(text):
    out = {}
    m = re.search(r"Number of cells:\s+(\d+)", text)
    if m: out["cells"] = int(m.group(1))
    m = re.search(r"Chip area for module.*?:\s+([\d.]+)", text)
    if m: out["area_um2"] = float(m.group(1))
    return out

def main(argv=None):
    ap = argparse.ArgumentParser()
    ap.add_argument("--macro", required=True)
    ap.add_argument("--commit", required=True)
    ap.add_argument("--stat", required=True)
    ap.add_argument("--out", required=True)
    a = ap.parse_args(argv)
    with open(a.stat) as f: metrics = parse_yosys_stat(f.read())
    doc = {"board": "gf180_j4mmu", "variant": "j4-rom",
           "macro": a.macro, "commit": a.commit, "metrics": metrics}
    with open(a.out, "w") as f: json.dump(doc, f, indent=2)
    return 0

if __name__ == "__main__":
    sys.exit(main())
