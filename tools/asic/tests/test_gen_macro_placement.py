# tools/asic/tests/test_gen_macro_placement.py
import sys
import tempfile
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))

from gen_macro_placement import (
    die_core_for_grid,
    gen_placement,
    instances_from_prefix,
    lef_dimensions,
)

SAMPLE_LEF = """
VERSION 5.7 ;
MACRO gf180mcu_fd_ip_sram__sram512x8m8wm1
  CLASS BLOCK ;
  SIZE 448.42 BY 504.88 ;
  ORIGIN 0.0 0.0 ;
END gf180mcu_fd_ip_sram__sram512x8m8wm1
"""


def test_lef_dimensions_parses_size():
    with tempfile.TemporaryDirectory() as d:
        p = Path(d) / "m.lef"
        p.write_text(SAMPLE_LEF)
        w, h = lef_dimensions(p)
        assert w == 448.42
        assert h == 504.88


def test_instances_from_prefix():
    names = instances_from_prefix("tile", 4)
    assert names == ["tile0", "tile1", "tile2", "tile3"]


def test_gen_placement_2x2_grid_pitch():
    # 4 macros, 2 cols -> 2x2 grid; pitch = width/height + 20um channel.
    names = instances_from_prefix("tile", 4)
    placement = gen_placement(
        names, width=100.0, height=200.0, cols=2, pitch_x=120.0, pitch_y=220.0,
        origin_x=50.0, origin_y=50.0,
    )
    assert list(placement.keys()) == names
    assert placement["tile0"] == {"location": [50.0, 50.0], "orientation": "N"}
    assert placement["tile1"] == {"location": [170.0, 50.0], "orientation": "N"}
    assert placement["tile2"] == {"location": [50.0, 270.0], "orientation": "N"}
    assert placement["tile3"] == {"location": [170.0, 270.0], "orientation": "N"}


def test_gen_placement_rejects_overlapping_pitch():
    names = instances_from_prefix("tile", 2)
    try:
        gen_placement(names, width=100.0, height=100.0, cols=2, pitch_x=50.0, pitch_y=100.0)
        assert False, "expected ValueError for pitch smaller than footprint"
    except ValueError:
        pass


def test_die_core_for_grid_2x2():
    d = die_core_for_grid(
        n_instances=4, width=100.0, height=200.0, cols=2, pitch_x=120.0, pitch_y=220.0,
        origin_x=50.0, origin_y=50.0, margin=50.0,
    )
    # far edge of last (bottom-right) macro: x=170+100=270, y=270+200=470
    assert d["DIE_AREA"] == [0, 0, 320.0, 520.0]
    assert d["CORE_AREA"][2] < d["DIE_AREA"][2]
    assert d["CORE_AREA"][3] < d["DIE_AREA"][3]


def test_gen_placement_orientation_override():
    names = instances_from_prefix("t", 1)
    placement = gen_placement(names, 10, 10, cols=1, pitch_x=10, pitch_y=10, orientation="FN")
    assert placement["t0"]["orientation"] == "FN"
