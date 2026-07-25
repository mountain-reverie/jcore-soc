import json
import os
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, os.path.dirname(HERE))
import to_gha_bench  # noqa: E402


def _write(dirpath, board, metrics):
    p = os.path.join(dirpath, "%s.json" % board)
    with open(p, "w") as f:
        json.dump({"target": "ecp5-lfe5u-85f", "board": board, "commit": "c",
                   "metrics": metrics}, f)
    return p


class TestConvert(unittest.TestCase):
    def test_splits_by_direction_and_prefixes_target(self):
        m = [
            {"name": "ulx3s/LUT4", "unit": "cells", "value": 5111, "dir": "smaller"},
            {"name": "ulx3s/Fmax", "unit": "MHz", "value": 78.2, "dir": "bigger"},
        ]
        with tempfile.TemporaryDirectory() as d:
            size, speed = to_gha_bench.convert([_write(d, "ulx3s", m)])
        size_names = {e["name"]: e for e in size}
        speed_names = {e["name"]: e for e in speed}
        self.assertIn("ecp5-lfe5u-85f · ulx3s/LUT4", size_names)
        self.assertIn("ecp5-lfe5u-85f · ulx3s/Fmax", speed_names)
        self.assertEqual(size_names["ecp5-lfe5u-85f · ulx3s/LUT4"]["unit"], "cells")
        self.assertEqual(size_names["ecp5-lfe5u-85f · ulx3s/LUT4"]["value"], 5111)
        self.assertEqual(size_names["ecp5-lfe5u-85f · ulx3s/LUT4"]["extra"], "ulx3s")

    def test_deterministic_order(self):
        m = [{"name": "ulx3s/%s" % n, "unit": "cells", "value": 1, "dir": "smaller"}
             for n in ("MULT18X18D", "DP16KD", "LUT4")]
        with tempfile.TemporaryDirectory() as d:
            size, _ = to_gha_bench.convert([_write(d, "ulx3s", m)])
        names = [e["name"] for e in size]
        self.assertEqual(names, sorted(names))

    def test_gf180_die_series_and_kianv_limit_line_charted(self):
        # tools/asic/emit_die_metrics.py's canonical doc shape (Task 8): die
        # area series for the padded top + the pinned KianV reference
        # constant must convert into chartable customSmallerIsBetter
        # entries, same as any other board's metrics.
        m = [
            {"name": "gf180-die-area-mm2", "unit": "mm2", "value": 21.7194, "dir": "smaller"},
            {"name": "gf180-core-area-mm2", "unit": "mm2", "value": 21.3249, "dir": "smaller"},
            {"name": "kianv-die-20.1mm2", "unit": "mm2", "value": 20.1, "dir": "smaller"},
        ]
        with tempfile.TemporaryDirectory() as d:
            p = os.path.join(d, "metrics-die.json")
            with open(p, "w") as f:
                json.dump({"target": "gf180mcu-mcu7t5v0", "board": "gf180_j4mmu",
                           "commit": "c", "metrics": m}, f)
            size, speed = to_gha_bench.convert([p])
        size_names = {e["name"]: e for e in size}
        self.assertIn("gf180mcu-mcu7t5v0 · gf180-die-area-mm2", size_names)
        self.assertIn("gf180mcu-mcu7t5v0 · gf180-core-area-mm2", size_names)
        self.assertIn("gf180mcu-mcu7t5v0 · kianv-die-20.1mm2", size_names)
        self.assertEqual(size_names["gf180mcu-mcu7t5v0 · kianv-die-20.1mm2"]["value"], 20.1)
        self.assertEqual(len(speed), 0)

    def test_multiple_boards_distinct_series(self):
        with tempfile.TemporaryDirectory() as d:
            paths = [
                _write(d, "ulx3s", [{"name": "ulx3s/LUT4", "unit": "cells", "value": 1, "dir": "smaller"}]),
                _write(d, "ecpix5", [{"name": "ecpix5/LUT4", "unit": "cells", "value": 2, "dir": "smaller"}]),
            ]
            size, _ = to_gha_bench.convert(paths)
        names = {e["name"] for e in size}
        self.assertIn("ecp5-lfe5u-85f · ulx3s/LUT4", names)
        self.assertIn("ecp5-lfe5u-85f · ecpix5/LUT4", names)


if __name__ == "__main__":
    unittest.main()
