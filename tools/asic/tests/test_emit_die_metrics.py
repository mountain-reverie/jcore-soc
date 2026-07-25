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
