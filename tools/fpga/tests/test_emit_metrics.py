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

    def test_fmax_last_per_clock(self):
        # nextpnr prints an early estimate then the final post-route value for
        # the same clock; the last (final) wins. The CPU domain is named with an
        # unstable yosys net id ('$glbnet$clk:6' -> 'clk:6').
        got = emit_metrics.parse_nextpnr_log(read("nextpnr_ecp5.log"))
        self.assertAlmostEqual(got["fmax"]["clk:6"], 34.64)
        self.assertAlmostEqual(got["fmax"]["clk_25mhz"], 412.00)

    def test_timing_pass(self):
        # no "(FAIL at" marker -> timing met
        self.assertFalse(emit_metrics.timing_failed(read("nextpnr_ecp5.log")))

    def test_timing_fail_detected(self):
        text = "Info: Max frequency for clock '$glbnet$clk_cpu': 18.0 MHz (FAIL at 25.00 MHz)\n"
        self.assertTrue(emit_metrics.timing_failed(text))


class TestParseCriticalPath(unittest.TestCase):
    def test_binding_clock_final_setup_path(self):
        crit = emit_metrics.parse_critical_path(read("nextpnr_critpath.log"))
        # slow_clk (20 MHz) is binding over fast_clk (200 MHz).
        self.assertEqual(crit["clock"], "slow_clk")
        self.assertAlmostEqual(crit["mhz"], 20.00)
        # The LAST setup block for the binding clock (not the superseded 99 MHz
        # estimate) and not the trailing cross-domain '<async>' block.
        self.assertEqual(crit["source"], "real_src_TRELLIS_FF_Q.Q")
        self.assertEqual(crit["sink"], "real_sink_ram.CEB")
        self.assertAlmostEqual(crit["logic_ns"], 1.2)
        self.assertAlmostEqual(crit["routing_ns"], 8.0)

    def test_none_when_absent(self):
        self.assertIsNone(emit_metrics.parse_critical_path("no timing here\n"))

    def test_rides_along_in_doc(self):
        parsed = emit_metrics.parse_nextpnr_log(read("nextpnr_ecp5.log"))
        parsed["critical_path"] = {"clock": "x", "mhz": 1.0, "source": "s",
                                   "sink": "k", "logic_ns": 0.1, "routing_ns": 0.2}
        doc = emit_metrics.build_ecp5(parsed, board="ulx3s", commit="c")
        self.assertEqual(doc["critical_path"]["sink"], "k")
        # Absent when not parsed.
        doc2 = emit_metrics.build_ecp5(
            emit_metrics.parse_nextpnr_log(read("nextpnr_ecp5.log")),
            board="ulx3s", commit="c")
        self.assertNotIn("critical_path", doc2)


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
        # Fmax = the lowest (binding) constrained clock's final value (the CPU
        # domain clk:6 at 34.64, below the 412 input osc), bigger-is-better.
        self.assertAlmostEqual(names["ulx3s/Fmax"]["value"], 34.64)
        self.assertEqual(names["ulx3s/Fmax"]["dir"], "bigger")
        self.assertEqual(names["ulx3s/Fmax"]["unit"], "MHz")

    def test_omits_absent_metrics(self):
        doc = emit_metrics.build_ecp5({"util": {"TRELLIS_COMB": 10}, "fmax": {}},
                                      board="b", commit="c")
        names = [m["name"] for m in doc["metrics"]]
        self.assertEqual(names, ["b/LUT4"])


if __name__ == "__main__":
    unittest.main()
