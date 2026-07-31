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

# newer-yosys netlist: the .rows:1.genram_*.mem. labels are elided
NETLIST_ELIDED = r"""
module icache_adapter (a, b);
  gf180mcu_fd_ip_sram__sram64x8m8wm1 \u_ucache_ram.tag.subword_gen:1.sram_i  (.CLK(x));
  gf180mcu_fd_ip_sram__sram64x8m8wm1 \u_ucache_ram.tag.subword_gen:2.sram_i  (.CLK(y));
endmodule
"""


def test_canonical_ignores_version_varying_labels():
    long = "u_ucache_ram.tag.rows:1.genram_3x8x64.mem.subword_gen:1.sram_i"
    short = "u_ucache_ram.tag.subword_gen:1.sram_i"
    assert canonical(long) == canonical(short) == (("tag", ""), ("sub", "1"))


def test_remap_rewrites_keys_and_preserves_locations():
    cfg = json.loads(json.dumps(CFG))  # deep copy
    mods, n = remap_config(cfg, NETLIST_ELIDED)
    assert (mods, n) == (1, 2)
    insts = cfg["MACROS"]["gf180mcu_fd_ip_sram__sram64x8m8wm1"]["instances"]
    # keys are now the netlist's (short) names
    assert set(insts) == {
        "u_ucache_ram.tag.subword_gen:1.sram_i",
        "u_ucache_ram.tag.subword_gen:2.sram_i",
    }
    # locations follow their semantic macro (subword_gen:1 -> 50.0)
    assert insts["u_ucache_ram.tag.subword_gen:1.sram_i"]["location"] == [50.0, 35.0]
    assert insts["u_ucache_ram.tag.subword_gen:2.sram_i"]["location"] == [536.86, 35.0]


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
