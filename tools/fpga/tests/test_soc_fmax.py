import os
import sys
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, os.path.dirname(HERE))  # import tools/fpga/soc_fmax.py
import soc_fmax  # noqa: E402


# Shape taken from a real targets/boards/ulx3s/build/metrics.json. Note the
# variant-tagged metric name, and that `critical_path` is ABSENT rather than
# null when nextpnr's report could not be parsed -- emit_metrics only sets the
# key when it has a value, so the reader must not assume it exists.
DOC = {
    "target": "ulx3s",
    "board": "ulx3s",
    "commit": "deadbeef",
    "metrics": [
        {"name": "ulx3s [j4-rom]/TRELLIS_COMB", "unit": "cells", "value": 30000, "dir": "smaller"},
        {"name": "ulx3s [j4-rom]/Fmax", "unit": "MHz", "value": 25.11, "dir": "bigger"},
    ],
}

DOC_WITH_PATH = dict(DOC, critical_path={
    "clock": "$glbnet$clk", "mhz": 25.11,
    "source": "cpu0.u_datapath.reg_FF.Q", "sink": "cpu0.u_cache.ram_CEB",
    "logic_ns": 0.8, "routing_ns": 5.2,
})


class TestFmaxFromMetrics(unittest.TestCase):

    def test_reads_the_fmax_metric(self):
        self.assertAlmostEqual(soc_fmax.fmax_from_metrics(DOC), 25.11)

    def test_ignores_non_fmax_metrics(self):
        # TRELLIS_COMB is 30000; a naive "first metric" reader would return it.
        self.assertNotEqual(soc_fmax.fmax_from_metrics(DOC), 30000)

    def test_missing_fmax_raises(self):
        doc = {"metrics": [{"name": "ulx3s [j4-rom]/TRELLIS_COMB", "value": 1}]}
        with self.assertRaises(ValueError):
            soc_fmax.fmax_from_metrics(doc)

    def test_empty_metrics_raises(self):
        with self.assertRaises(ValueError):
            soc_fmax.fmax_from_metrics({"metrics": []})

    def test_ambiguous_fmax_raises(self):
        # Two Fmax series would mean the caller cannot know which variant it got.
        doc = {"metrics": [
            {"name": "ulx3s [a]/Fmax", "value": 30.0},
            {"name": "ulx3s [b]/Fmax", "value": 40.0},
        ]}
        with self.assertRaises(ValueError):
            soc_fmax.fmax_from_metrics(doc)


class TestCriticalPath(unittest.TestCase):

    def test_absent_key_returns_none(self):
        self.assertIsNone(soc_fmax.critical_path_from_metrics(DOC))

    def test_present_key_returned(self):
        cp = soc_fmax.critical_path_from_metrics(DOC_WITH_PATH)
        self.assertEqual(cp["sink"], "cpu0.u_cache.ram_CEB")
        self.assertAlmostEqual(cp["routing_ns"], 5.2)


class TestStats(unittest.TestCase):

    def test_summarize_odd(self):
        s = soc_fmax.summarize([30.0, 25.0, 28.0])
        self.assertEqual((s["min"], s["median"], s["max"], s["n"]), (25.0, 28.0, 30.0, 3))

    def test_summarize_even_averages_middle(self):
        self.assertEqual(soc_fmax.summarize([10.0, 20.0, 30.0, 40.0])["median"], 25.0)

    def test_summarize_empty_raises(self):
        with self.assertRaises(ValueError):
            soc_fmax.summarize([])

    def test_noise_band(self):
        self.assertAlmostEqual(soc_fmax.noise_band([25.0, 28.0, 30.0]), 5.0)


if __name__ == "__main__":
    unittest.main()
