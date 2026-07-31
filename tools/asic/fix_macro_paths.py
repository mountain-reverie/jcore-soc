#!/usr/bin/env python3
"""Rewrite hand-placed vendor-SRAM macro instance paths in a (merged) LibreLane
config to match the ACTUAL instance names in a freshly-generated netlist.

Why: the gf180 cache configs (icache/dcache, 2K/4K/8K) hand-place each
gf180mcu_fd_ip_sram macro at a fixed location keyed by its full hierarchical
instance path. Those paths carry ghdl-yosys generate-block labels
(`rows:1`, `genram_3x8x64`, `mem`, `ram_s`, ...) whose presence/spelling
VARIES BY YOSYS VERSION -- a single-branch generate-if that yosys 0.44 keeps as
`.rows:1.genram_3x8x64.mem.` a newer yosys elides. When the config's paths and
the netlist's paths disagree, LibreLane reports "No macro instance <path> found"
and quits, so the cache never hardens.

The fix is version-independent: match config placements to netlist instances by
their SEMANTIC coordinates -- the tokens that actually identify a macro
(`tag`/`tag0`/`tag1`, `ram:N`, `col_gen:M`, `subword_gen:K`) -- and ignore the
structural label noise that differs between versions. Then rewrite the config's
instance keys to the netlist's actual names, preserving each location.

Operates on the MERGED config (run.sh's config.merged.json), so the committed
config.json is never mutated and the result always matches whatever yosys the
runner happens to ship.

Usage: fix_macro_paths.py <merged_config.json> <netlist.v>
No-op (exit 0) if there are no vendor-SRAM MACROS or they already match.
"""
import json
import re
import sys
from pathlib import Path

SRAM_MODULE_RE = re.compile(r"gf180mcu_fd_ip_sram__")

# semantic tokens (anchored, per '.'-split token). Accept `:N` or `[N]` index
# forms so a future yosys index-syntax change still matches.
_TOKEN_RES = [
    ("tag", re.compile(r"^tag(\d*)$")),                       # tag / tag0 / tag1
    ("ram", re.compile(r"^ram[:\[](\d+)\]?$")),               # data bank
    ("col", re.compile(r"^col_gen[:\[](\d+)\]?$")),           # data column
    ("sub", re.compile(r"^subword_gen[:\[](\d+)\]?$")),       # tag lane
]


def canonical(path):
    """Semantic signature of an instance path: the ordered (kind, index) tokens
    that identify the macro, ignoring version-varying structural labels."""
    key = []
    for tok in path.strip().lstrip("\\").split("."):
        for kind, rx in _TOKEN_RES:
            m = rx.match(tok)
            if m:
                key.append((kind, m.group(1)))
                break
    return tuple(key)


def netlist_instances(netlist_text, module):
    """All instance names of `module` in the netlist (escaped id -> bare name)."""
    # verilog: `  <module> \<name with dots and colons>  (`  (escaped id ends at ws)
    rx = re.compile(r"^\s*" + re.escape(module) + r"\s+\\(\S+)\s*\(", re.M)
    return [m.group(1) for m in rx.finditer(netlist_text)]


def remap_config(cfg, netlist_text):
    """Rewrite cfg['MACROS'][sram].instances keys to the netlist's actual names.
    Returns (n_modules_fixed, n_instances_remapped). Raises on a real mismatch."""
    fixed_mods = 0
    remapped = 0
    for module, mv in cfg.get("MACROS", {}).items():
        if not SRAM_MODULE_RE.search(module):
            continue  # child-macro LEFs (top/pad_ring) -- leave untouched
        insts = mv.get("instances", {})
        if not insts:
            continue
        actual = netlist_instances(netlist_text, module)
        # map canonical signature -> actual name
        actual_by_key = {}
        for name in actual:
            actual_by_key.setdefault(canonical(name), name)
        new_insts = {}
        changed = False
        for path, place in insts.items():
            k = canonical(path)
            hit = actual_by_key.get(k)
            if hit is None:
                raise SystemExit(
                    f"fix_macro_paths: {module}: config instance {path!r} "
                    f"(signature {k}) has no match among {len(actual)} netlist "
                    f"instances {actual!r} -- placement/netlist disagree.")
            if hit != path:
                changed = True
                remapped += 1
            new_insts[hit] = place
        if len(new_insts) != len(insts):
            raise SystemExit(
                f"fix_macro_paths: {module}: {len(insts)} placements collapsed "
                f"to {len(new_insts)} after remap (duplicate signatures?)")
        if len(actual) != len(insts):
            raise SystemExit(
                f"fix_macro_paths: {module}: netlist has {len(actual)} SRAM "
                f"instances but config places {len(insts)} -- count mismatch.")
        if changed:
            mv["instances"] = new_insts
            fixed_mods += 1
    return fixed_mods, remapped


def main(argv=None):
    argv = argv if argv is not None else sys.argv[1:]
    if len(argv) != 2:
        sys.exit("usage: fix_macro_paths.py <merged_config.json> <netlist.v>")
    cfg_path, net_path = argv
    cfg = json.loads(Path(cfg_path).read_text())
    if not any(SRAM_MODULE_RE.search(m) for m in cfg.get("MACROS", {})):
        print("fix_macro_paths: no vendor-SRAM MACROS -- nothing to do.")
        return 0
    net = Path(net_path).read_text()
    mods, n = remap_config(cfg, net)
    if n:
        Path(cfg_path).write_text(json.dumps(cfg, indent=2) + "\n")
        print(f"fix_macro_paths: remapped {n} SRAM placement(s) across {mods} "
              f"module(s) to the netlist's actual instance names.")
    else:
        print("fix_macro_paths: SRAM placements already match the netlist.")
    return 0


if __name__ == "__main__":
    sys.exit(main())
