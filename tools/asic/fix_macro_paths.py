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
# forms -- yosys 0.44 emits `subword_gen:1` (1-based colon), newer yosys emits
# `subword_gen[0]` (0-based bracket); both must map to the same macro.
_KIND_ORDER = {"tag": 0, "ram": 1, "col": 2, "sub": 3}
_TOKEN_RES = [
    ("tag", re.compile(r"^tag(\d*)$")),                       # tag / tag0 / tag1
    ("ram", re.compile(r"^ram[:\[](\d+)\]?$")),               # data bank
    ("col", re.compile(r"^col_gen[:\[](\d+)\]?$")),           # data column
    ("sub", re.compile(r"^subword_gen[:\[](\d+)\]?$")),       # tag lane
]


def canonical(path):
    """Semantic SORT KEY of an instance path: the ordered (kind_rank, index)
    tokens that identify the macro, ignoring version-varying structural labels.

    The index is kept as an int and used only for RELATIVE ordering, never
    matched literally -- so the 1-based-colon vs 0-based-bracket difference
    between yosys versions is a uniform offset that sorted-rank matching sees
    through. Instances of one module are then paired by ascending sort key."""
    key = []
    for tok in path.strip().lstrip("\\").split("."):
        for kind, rx in _TOKEN_RES:
            m = rx.match(tok)
            if m:
                idx = int(m.group(1)) if m.group(1) else 0
                key.append((_KIND_ORDER[kind], idx))
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
        if len(actual) != len(insts):
            raise SystemExit(
                f"fix_macro_paths: {module}: netlist has {len(actual)} SRAM "
                f"instances but config places {len(insts)} -- count mismatch "
                f"(netlist: {actual!r}).")
        # Pair config placements to netlist instances by ASCENDING semantic sort
        # key. Both sides enumerate the same generate structure, so rank-K on
        # each side is the same logical macro regardless of the version's index
        # base/separator. A duplicate sort key on either side is a real ambiguity.
        cfg_sorted = sorted(insts.items(), key=lambda kv: canonical(kv[0]))
        net_sorted = sorted(actual, key=canonical)
        cfg_keys = [canonical(p) for p, _ in cfg_sorted]
        net_keys = [canonical(n) for n in net_sorted]
        if len(set(cfg_keys)) != len(cfg_keys):
            raise SystemExit(f"fix_macro_paths: {module}: duplicate config "
                             f"sort keys {cfg_keys} -- cannot pair unambiguously.")
        if len(set(net_keys)) != len(net_keys):
            raise SystemExit(f"fix_macro_paths: {module}: duplicate netlist "
                             f"sort keys {net_keys} -- cannot pair unambiguously.")
        new_insts = {}
        changed = False
        for (path, place), name in zip(cfg_sorted, net_sorted):
            new_insts[name] = place
            if name != path:
                changed = True
                remapped += 1
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
