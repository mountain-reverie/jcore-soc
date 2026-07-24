#!/usr/bin/env python3
"""gen_macro_placement.py -- generate a LibreLane MACROS{...}.instances{}
grid placement (+ a die/core size that fits it) for a large array of
identical hard-IP macros (vendor SRAM tiles, etc), so hardening blocks with
dozens of macros (bootram's 32x gf180mcu_fd_ip_sram__sram512x8m8wm1, Task 6's
top-level integration) doesn't require hand-placing each instance (as
Task 4's dcache/icache ~20-instance MACROS.instances blocks did).

Given:
  - a macro's footprint (parsed from its .lef, or passed as --width/--height)
  - a list of instance (hierarchical) names to place
  - a grid spec: rows x cols, pitch (channel spacing added to the macro's own
    footprint) and an origin

...emits the `instances: {...}` dict LibreLane's MACROS[<cellname>] config
expects (location + orientation per instance), row-major, plus a die/core
size that fits the grid with a margin.

Reused by:
  - targets/asic/gf180_j4mmu/librelane/bootram/config.json (Task 5, this
    generator's validating case: 32x sram512x8m8wm1, 4 lanes x 8 tiles)
  - Task 6's top-level integration (many more macros: j4_core/icache/dcache/
    bootram placed as blackboxes alongside soc_cluster's flattened glue)

Usage (CLI, for generating a config snippet by hand):
  python3 gen_macro_placement.py \\
      --lef path/to/gf180mcu_fd_ip_sram__sram512x8m8wm1.lef \\
      --instances-file names.txt \\
      --cols 4 --pitch-x 20 --pitch-y 20 --origin-x 50 --origin-y 50 \\
      --margin 50

Or import and call `gen_placement(...)` / `lef_dimensions(...)` directly.
"""
from __future__ import annotations

import argparse
import json
import re
import sys
from pathlib import Path
from typing import Iterable, Sequence


def lef_dimensions(lef_path: str | Path) -> tuple[float, float]:
    """Parse SIZE <w> BY <h> ; out of a LEF MACRO block. Returns (width, height)
    in the LEF's native units (microns, matching LibreLane's location/DIE_AREA
    convention)."""
    text = Path(lef_path).read_text()
    m = re.search(r"SIZE\s+([0-9.]+)\s+BY\s+([0-9.]+)\s*;", text)
    if not m:
        raise ValueError(f"no SIZE ... BY ... ; found in {lef_path}")
    return float(m.group(1)), float(m.group(2))


def gen_placement(
    instances: Sequence[str],
    width: float,
    height: float,
    cols: int,
    pitch_x: float,
    pitch_y: float,
    origin_x: float = 50.0,
    origin_y: float = 50.0,
    orientation: str = "N",
) -> dict[str, dict]:
    """Lay `instances` out row-major into a grid of `cols` columns, each cell
    spaced `pitch_x`/`pitch_y` apart (pitch is the MACRO-TO-MACRO stride, i.e.
    must be >= width/height respectively to avoid overlap -- the caller should
    add channel/PDN-strap spacing on top of the raw macro footprint, same as
    the hand-placed dcache/icache configs' ~452um stride vs ~448um macro
    width). Returns the `instances` sub-dict LibreLane's
    MACROS[<cellname>].instances expects: {name: {location: [x, y],
    orientation: ...}}.
    """
    if cols <= 0:
        raise ValueError("cols must be >= 1")
    if pitch_x < width or pitch_y < height:
        raise ValueError(
            f"pitch ({pitch_x}x{pitch_y}) smaller than macro footprint "
            f"({width}x{height}) -- instances would overlap"
        )
    out: dict[str, dict] = {}
    for idx, name in enumerate(instances):
        row, col = divmod(idx, cols)
        x = origin_x + col * pitch_x
        y = origin_y + row * pitch_y
        out[name] = {"location": [round(x, 2), round(y, 2)], "orientation": orientation}
    return out


def die_core_for_grid(
    n_instances: int,
    width: float,
    height: float,
    cols: int,
    pitch_x: float,
    pitch_y: float,
    origin_x: float = 50.0,
    origin_y: float = 50.0,
    margin: float = 50.0,
) -> dict[str, list]:
    """Compute a DIE_AREA/CORE_AREA that fits the grid produced by
    gen_placement with `margin` clearance beyond the last macro's far edge,
    for FP_SIZING: absolute floorplans (matching the dcache/icache/j4_core
    convention)."""
    rows = (n_instances + cols - 1) // cols
    last_col = min(cols, n_instances) - 1
    last_row = rows - 1
    far_x = origin_x + last_col * pitch_x + width
    far_y = origin_y + last_row * pitch_y + height
    die_w = far_x + margin
    die_h = far_y + margin
    return {
        "DIE_AREA": [0, 0, round(die_w, 2), round(die_h, 2)],
        "CORE_AREA": [
            round(margin / 5, 2),
            round(margin / 5, 2),
            round(die_w - margin / 5, 2),
            round(die_h - margin / 5, 2),
        ],
    }


def instances_from_prefix(prefix: str, count: int, start: int = 0) -> list[str]:
    """Convenience: `<prefix><i>` for i in [start, start+count)."""
    return [f"{prefix}{i}" for i in range(start, start + count)]


def main(argv: list[str] | None = None) -> int:
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("--lef", help="macro .lef to parse width/height from")
    ap.add_argument("--width", type=float, help="macro width (overrides --lef)")
    ap.add_argument("--height", type=float, help="macro height (overrides --lef)")
    ap.add_argument("--instances-file", help="file with one instance name per line")
    ap.add_argument("--instance-prefix", help="generate names as <prefix><i>")
    ap.add_argument("--count", type=int, help="count for --instance-prefix")
    ap.add_argument("--cols", type=int, required=True)
    ap.add_argument("--pitch-x", type=float, required=True)
    ap.add_argument("--pitch-y", type=float, required=True)
    ap.add_argument("--origin-x", type=float, default=50.0)
    ap.add_argument("--origin-y", type=float, default=50.0)
    ap.add_argument("--margin", type=float, default=50.0)
    ap.add_argument("--orientation", default="N")
    ap.add_argument("--cell-name", help="MACROS key to nest instances under")
    args = ap.parse_args(argv)

    if args.width is not None and args.height is not None:
        width, height = args.width, args.height
    elif args.lef:
        width, height = lef_dimensions(args.lef)
    else:
        ap.error("need --lef or --width/--height")

    if args.instances_file:
        instances = [
            l.strip() for l in Path(args.instances_file).read_text().splitlines() if l.strip()
        ]
    elif args.instance_prefix and args.count:
        instances = instances_from_prefix(args.instance_prefix, args.count)
    else:
        ap.error("need --instances-file or --instance-prefix/--count")

    placement = gen_placement(
        instances, width, height, args.cols, args.pitch_x, args.pitch_y,
        args.origin_x, args.origin_y, args.orientation,
    )
    die_core = die_core_for_grid(
        len(instances), width, height, args.cols, args.pitch_x, args.pitch_y,
        args.origin_x, args.origin_y, args.margin,
    )
    result: dict = {"instances": placement, **die_core}
    if args.cell_name:
        result = {"MACROS": {args.cell_name: {"instances": placement}}, **die_core}
    json.dump(result, sys.stdout, indent=2)
    sys.stdout.write("\n")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
