# tools/asic/tests/test_emit_die_metrics.py
import json
import sys
import tempfile
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))

from emit_die_metrics import KIANV_DIE_MM2, build_die_doc, parse_or_metrics

SAMPLE_METRICS = {
    "design__die__area": 21719400.0,
    "design__core__area": 21324900.0,
    "design__instance__area": 13915900.0,
    "design__instance__utilization": 0.652565,
}

SAMPLE_MACRO_METRICS = {
    "design__die__area": 1222260.0,
    "design__core__area": 1171760.0,
    "design__instance__area": 547577.0,
}


def _write(d, name, data):
    p = Path(d) / name
    p.write_text(json.dumps(data))
    return p


def test_parse_or_metrics_extracts_die_core_instance_um2():
    with tempfile.TemporaryDirectory() as d:
        p = _write(d, "metrics.json", SAMPLE_METRICS)
        m = parse_or_metrics(p)
        assert m["die_um2"] == 21719400.0
        assert m["core_um2"] == 21324900.0
        assert m["instance_um2"] == 13915900.0


def test_build_die_doc_converts_um2_to_mm2_and_includes_kianv_constant():
    with tempfile.TemporaryDirectory() as d:
        top = _write(d, "top.json", SAMPLE_METRICS)
        j4 = _write(d, "j4_core.json", SAMPLE_MACRO_METRICS)
        doc = build_die_doc(
            top_metrics=top,
            macro_metrics={"j4_core": j4},
            commit="deadbeef",
        )
        assert doc["target"] == "gf180mcu-mcu7t5v0"
        assert doc["board"] == "gf180_j4mmu"
        assert doc["commit"] == "deadbeef"

        by_name = {m["name"]: m for m in doc["metrics"]}

        die = by_name["gf180-die-area-mm2"]
        assert die["value"] == 21.7194
        assert die["unit"] == "mm2"
        assert die["dir"] == "smaller"

        core = by_name["gf180-core-area-mm2"]
        assert core["value"] == 21.3249

        placed = by_name["gf180-placed-silicon-mm2"]
        assert placed["value"] == 13.9159

        macro = by_name["gf180-die-area-mm2 [j4_core]"]
        assert macro["value"] == 1.22226

        kianv = by_name["kianv-die-20.1mm2"]
        assert kianv["value"] == KIANV_DIE_MM2 == 20.1
        assert kianv["dir"] == "smaller"


def test_build_die_doc_always_emits_kianv_even_without_top():
    with tempfile.TemporaryDirectory() as d:
        j4 = _write(d, "j4_core.json", SAMPLE_MACRO_METRICS)
        doc = build_die_doc(top_metrics=None, macro_metrics={"j4_core": j4}, commit="c0ffee")
        names = {m["name"] for m in doc["metrics"]}
        assert "kianv-die-20.1mm2" in names
        assert "gf180-die-area-mm2" not in names
        assert "gf180-die-area-mm2 [j4_core]" in names


# route.tcl writes design__die__area in um2 (width_um * height_um); 4120x4242.
SAMPLE_PADDED = {
    "design__die__width_um": 4120.0,
    "design__die__height_um": 4242.0,
    "design__die__area": 4120.0 * 4242.0,
    "design__route__drc_errors": 0,
    "design__route__wire_segments": 9468,
}


# chip_top (LibreLane Chip flow) final/metrics.json: the whole chip -- flat soc
# + abutted IO pad ring -- in ONE run. Note the DRC key is `route__drc_errors`,
# NOT the `design__route__drc_errors` that pad_ring's route.tcl wrote.
SAMPLE_CHIP_TOP = {
    "design__die__area": 12916800.0,      # 2700 x 4784
    "design__core__area": 7073120.0,
    "route__drc_errors": 0,
}


def test_build_die_doc_chip_top_feeds_the_padded_die_series():
    """chip_top replaces pad_ring as the source of the headline padded die.

    It must land on the SAME series names so the dashboard history is
    continuous -- it is the same measurement (a complete die with its pad
    ring), produced by a flow that actually works.
    """
    with tempfile.TemporaryDirectory() as d:
        ct = _write(d, "metrics.json", SAMPLE_CHIP_TOP)
        doc = build_die_doc(top_metrics=None, macro_metrics={}, commit="beef",
                            chip_top=ct)
        by_name = {m["name"]: m for m in doc["metrics"]}
        padded = by_name["gf180-padded-die-mm2"]
        assert padded["value"] == round(12916800.0 / 1e6, 6)  # 12.9168
        assert padded["value"] < KIANV_DIE_MM2
        assert by_name["gf180-padded-die-drc"]["value"] == 0


def test_build_die_doc_chip_top_reports_nonzero_drc():
    """A dirty route must not be silently published as clean."""
    with tempfile.TemporaryDirectory() as d:
        ct = _write(d, "metrics.json", dict(SAMPLE_CHIP_TOP,
                                            **{"route__drc_errors": 7}))
        doc = build_die_doc(top_metrics=None, macro_metrics={}, commit="beef",
                            chip_top=ct)
        by_name = {m["name"]: m for m in doc["metrics"]}
        assert by_name["gf180-padded-die-drc"]["value"] == 7


def test_build_die_doc_emits_padded_die_below_kianv():
    with tempfile.TemporaryDirectory() as d:
        pad = _write(d, "padring_metrics.json", SAMPLE_PADDED)
        doc = build_die_doc(top_metrics=None, macro_metrics={}, commit="beef",
                            padded_die=pad)
        by_name = {m["name"]: m for m in doc["metrics"]}
        padded = by_name["gf180-padded-die-mm2"]
        assert padded["value"] == round(4120.0 * 4242.0 / 1e6, 6)  # ~17.48
        assert padded["value"] < KIANV_DIE_MM2  # the whole point
        assert by_name["gf180-padded-die-drc"]["value"] == 0
