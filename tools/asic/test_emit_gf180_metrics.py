# tools/asic/test_emit_gf180_metrics.py
from emit_gf180_metrics import parse_yosys_stat

SAMPLE = """
   Number of cells:               1234
     gf180mcu_fd_sc_mcu7t5v0__dffq_1   200
     gf180mcu_fd_sc_mcu7t5v0__nand2_1  400
   Chip area for module '\\j4_core': 287500.500000
"""

def test_parse_cells_and_area():
    m = parse_yosys_stat(SAMPLE)
    assert m["cells"] == 1234
    assert m["area_um2"] == 287500.5

def test_parse_missing_area_is_best_effort():
    m = parse_yosys_stat("   Number of cells:  10\n")
    assert m["cells"] == 10
    assert "area_um2" not in m
