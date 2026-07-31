import json
import sys
import tempfile
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))

from fix_macro_paths import canonical, remap_config

# yosys-0.44-style config paths (with the generate-block labels)
CFG = {
    "MACROS": {
        "gf180mcu_fd_ip_sram__sram64x8m8wm1": {
            "lef": ["dir::macros/x.lef"],
            "instances": {
                "u_ucache_ram.tag.rows:1.genram_3x8x64.mem.subword_gen:1.sram_i":
                    {"location": [50.0, 35.0], "orientation": "N"},
                "u_ucache_ram.tag.rows:1.genram_3x8x64.mem.subword_gen:2.sram_i":
                    {"location": [536.86, 35.0], "orientation": "N"},
            },
        },
    },
}

# newer-yosys netlist: labels elided AND 0-based BRACKET indices (the real CI
# case) -- subword_gen[0]/[1] vs the config's subword_gen:1/:2
NETLIST_ELIDED = r"""
module icache_adapter (a, b);
  gf180mcu_fd_ip_sram__sram64x8m8wm1 \u_ucache_ram.tag.subword_gen[0].sram_i  (.CLK(x));
  gf180mcu_fd_ip_sram__sram64x8m8wm1 \u_ucache_ram.tag.subword_gen[1].sram_i  (.CLK(y));
endmodule
"""


def test_canonical_is_version_invariant_sort_key():
    # colon-1-based, bracket-0-based, and label-elided all sort-compatibly:
    # the index is used for ORDER, not literal equality
    long = "u_ucache_ram.tag.rows:1.genram_3x8x64.mem.subword_gen:1.sram_i"
    short = "u_ucache_ram.tag.subword_gen:1.sram_i"
    bracket = "u_ucache_ram.tag.subword_gen[0].sram_i"
    assert canonical(long) == canonical(short) == ((0, 0), (3, 1))
    assert canonical(bracket) == ((0, 0), (3, 0))
    # same relative order (both are "the first tag lane")
    assert canonical(short) > canonical(bracket)  # 1 > 0, but rank-0 in each list


def test_remap_pairs_by_rank_across_index_base_and_separator():
    cfg = json.loads(json.dumps(CFG))  # deep copy
    mods, n = remap_config(cfg, NETLIST_ELIDED)
    assert (mods, n) == (1, 2)
    insts = cfg["MACROS"]["gf180mcu_fd_ip_sram__sram64x8m8wm1"]["instances"]
    # keys are now the netlist's 0-based bracket names
    assert set(insts) == {
        "u_ucache_ram.tag.subword_gen[0].sram_i",
        "u_ucache_ram.tag.subword_gen[1].sram_i",
    }
    # rank-0 config placement (subword:1 -> 50.0) pairs with netlist rank-0 ([0])
    assert insts["u_ucache_ram.tag.subword_gen[0].sram_i"]["location"] == [50.0, 35.0]
    assert insts["u_ucache_ram.tag.subword_gen[1].sram_i"]["location"] == [536.86, 35.0]


def test_remap_is_noop_when_names_already_match():
    # netlist that already uses the long names => no remap
    net = (r"  gf180mcu_fd_ip_sram__sram64x8m8wm1 "
           r"\u_ucache_ram.tag.rows:1.genram_3x8x64.mem.subword_gen:1.sram_i  ()"
           "\n  gf180mcu_fd_ip_sram__sram64x8m8wm1 "
           r"\u_ucache_ram.tag.rows:1.genram_3x8x64.mem.subword_gen:2.sram_i  ()")
    cfg = json.loads(json.dumps(CFG))
    mods, n = remap_config(cfg, net)
    assert (mods, n) == (0, 0)


def test_remap_errors_on_count_mismatch():
    cfg = json.loads(json.dumps(CFG))
    # netlist with only ONE of the two placed SRAMs
    net = (r"  gf180mcu_fd_ip_sram__sram64x8m8wm1 "
           r"\u_ucache_ram.tag.subword_gen:1.sram_i  ()")
    try:
        remap_config(cfg, net)
    except SystemExit:
        return
    raise AssertionError("expected SystemExit on count mismatch")
