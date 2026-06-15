import json
import os
import sys
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, os.path.dirname(HERE))  # import tools/fpga/emit_metrics.py
import emit_metrics  # noqa: E402

FIX = os.path.join(HERE, "fixtures")


def read(name):
    with open(os.path.join(FIX, name)) as f:
        return f.read()


class TestParseNextpnr(unittest.TestCase):
    def test_utilisation(self):
        got = emit_metrics.parse_nextpnr_log(read("nextpnr_ecp5.log"))
        self.assertEqual(got["util"]["TRELLIS_COMB"], 5111)
        self.assertEqual(got["util"]["TRELLIS_FF"], 975)
        self.assertEqual(got["util"]["DP16KD"], 16)
        self.assertEqual(got["util"]["MULT18X18D"], 2)
        self.assertEqual(got["util"]["EHXPLLL"], 1)
        self.assertEqual(got["util"]["TRELLIS_IO"], 21)

    def test_fmax_lowest_per_clock(self):
        got = emit_metrics.parse_nextpnr_log(read("nextpnr_ecp5.log"))
        self.assertAlmostEqual(got["fmax"]["clk_cpu"], 78.20)
        self.assertAlmostEqual(got["fmax"]["clk_25mhz"], 412.00)

    def test_timing_pass(self):
        # no "(FAIL at" marker -> timing met
        self.assertFalse(emit_metrics.timing_failed(read("nextpnr_ecp5.log")))

    def test_timing_fail_detected(self):
        text = "Info: Max frequency for clock '$glbnet$clk_cpu': 18.0 MHz (FAIL at 25.00 MHz)\n"
        self.assertTrue(emit_metrics.timing_failed(text))


class TestBuildDoc(unittest.TestCase):
    def test_canonical_doc(self):
        doc = emit_metrics.build_ecp5(
            emit_metrics.parse_nextpnr_log(read("nextpnr_ecp5.log")),
            board="ulx3s", commit="abc123")
        self.assertEqual(doc["target"], "ecp5-lfe5u-85f")
        self.assertEqual(doc["board"], "ulx3s")
        names = {m["name"]: m for m in doc["metrics"]}
        self.assertEqual(names["ulx3s/LUT4"]["value"], 5111)
        self.assertEqual(names["ulx3s/LUT4"]["dir"], "smaller")
        self.assertEqual(names["ulx3s/LUT4"]["unit"], "cells")
        self.assertEqual(names["ulx3s/FF"]["value"], 975)
        self.assertEqual(names["ulx3s/DP16KD"]["value"], 16)
        self.assertEqual(names["ulx3s/MULT18X18D"]["value"], 2)
        self.assertEqual(names["ulx3s/EHXPLLL"]["value"], 1)
        self.assertEqual(names["ulx3s/IO"]["value"], 21)
        # Fmax of the CPU clock domain, bigger-is-better
        self.assertAlmostEqual(names["ulx3s/Fmax clk_cpu"]["value"], 78.20)
        self.assertEqual(names["ulx3s/Fmax clk_cpu"]["dir"], "bigger")
        self.assertEqual(names["ulx3s/Fmax clk_cpu"]["unit"], "MHz")

    def test_omits_absent_metrics(self):
        doc = emit_metrics.build_ecp5({"util": {"TRELLIS_COMB": 10}, "fmax": {}},
                                      board="b", commit="c")
        names = [m["name"] for m in doc["metrics"]]
        self.assertEqual(names, ["b/LUT4"])


if __name__ == "__main__":
    unittest.main()
