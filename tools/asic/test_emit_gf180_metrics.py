# tools/asic/test_emit_gf180_metrics.py
import tempfile
from pathlib import Path

from emit_gf180_metrics import build_canonical_doc, parse_macros_list, parse_yosys_stat

SAMPLE = """
   Number of cells:               1234
     gf180mcu_fd_sc_mcu7t5v0__dffq_1   200
     gf180mcu_fd_sc_mcu7t5v0__nand2_1  400
   Chip area for module '\\j4_core': 287500.500000
     of which used for sequential elements:  94275.16 (32.79%)
"""

SAMPLE_NO_SEQ = """
   Number of cells:               1234
   Chip area for module '\\devices': 133729.000000
"""


def test_parse_cells_and_area():
    m = parse_yosys_stat(SAMPLE)
    assert m["cells"] == 1234
    assert m["area_um2"] == 287500.5


def test_parse_missing_area_is_best_effort():
    m = parse_yosys_stat("   Number of cells:  10\n")
    assert m["cells"] == 10
    assert "area_um2" not in m


def test_parse_seq_pct_present():
    m = parse_yosys_stat(SAMPLE)
    assert m["seq_area_um2"] == 94275.16
    assert m["seq_pct"] == 32.79


def test_parse_seq_pct_absent_is_best_effort():
    m = parse_yosys_stat(SAMPLE_NO_SEQ)
    assert m["cells"] == 1234
    assert "seq_pct" not in m
    assert "seq_area_um2" not in m


MACROS_LIST = """\
# macro        ghdl_top_entity   synth_top   inflated?
j4_core       cpu_synth_j4_rom  cpu         inflated
cache_ctrl.icache  icache       icache      inflated
soc_cluster.devices     devices     devices     -
top           soc         soc         inflated
"""


def test_parse_macros_list_inflated_column():
    with tempfile.TemporaryDirectory() as d:
        p = Path(d) / "macros.list"
        p.write_text(MACROS_LIST)
        inflated = parse_macros_list(p)
    assert inflated["j4_core"] is True
    assert inflated["cache_ctrl.icache"] is True
    assert inflated["soc_cluster.devices"] is False
    assert inflated["top"] is True


def test_build_canonical_doc_shape_and_dir():
    with tempfile.TemporaryDirectory() as d:
        macros_list = Path(d) / "macros.list"
        macros_list.write_text(MACROS_LIST)
        j4_stat = Path(d) / "j4_core.stat.txt"
        j4_stat.write_text(SAMPLE)
        dev_stat = Path(d) / "devices.stat.txt"
        dev_stat.write_text(SAMPLE_NO_SEQ)

        doc = build_canonical_doc(
            {"j4_core": str(j4_stat), "soc_cluster.devices": str(dev_stat)},
            commit="deadbeef",
            macros_list=macros_list,
            target="gf180mcu-mcu7t5v0",
            board="gf180_j4mmu",
        )

    assert doc["target"] == "gf180mcu-mcu7t5v0"
    assert doc["board"] == "gf180_j4mmu"
    assert doc["commit"] == "deadbeef"
    for m in doc["metrics"]:
        assert m["dir"] == "smaller"
    names = [m["name"] for m in doc["metrics"]]
    assert "gf180_j4mmu [j4_core incl-inferred-RAM]/cells" in names
    assert "gf180_j4mmu [soc_cluster.devices]/cells" in names
    # seq_pct travels in the doc even though it's not a charted metric.
    assert doc["macros"]["j4_core"]["seq_pct"] == 32.79
    assert doc["macros"]["j4_core"]["inflated"] is True
    assert doc["macros"]["soc_cluster.devices"]["inflated"] is False


def test_build_canonical_doc_omits_top_from_charted_metrics():
    with tempfile.TemporaryDirectory() as d:
        macros_list = Path(d) / "macros.list"
        macros_list.write_text(MACROS_LIST)
        top_stat = Path(d) / "top.stat.txt"
        top_stat.write_text(SAMPLE)

        doc = build_canonical_doc(
            {"top": str(top_stat)},
            commit="deadbeef",
            macros_list=macros_list,
            target="gf180mcu-mcu7t5v0",
            board="gf180_j4mmu",
        )

    assert doc["metrics"] == []
    # still traceable in the non-charted section, with the caveat flagged.
    assert doc["macros"]["top"]["inflated"] is True
