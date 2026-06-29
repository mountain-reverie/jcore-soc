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


class TestParseIce40(unittest.TestCase):
    def test_yosys_stat_lc_ram(self):
        """parse_yosys_stat maps real SB_LUT4/SB_RAM40_4K to canonical ICESTORM_* keys."""
        s = emit_metrics.parse_yosys_stat(read("yosys_stat_ice40.txt"))
        # SB_LUT4 -> ICESTORM_LC
        self.assertEqual(s["ICESTORM_LC"], 5443)
        # SB_RAM40_4K -> ICESTORM_RAM (count is 9 in the fixture)
        self.assertEqual(s["ICESTORM_RAM"], 9)

    def test_nextpnr_ice40_util_and_fmax(self):
        p = emit_metrics.parse_nextpnr_ice40_log(read("nextpnr_ice40.log"))
        self.assertEqual(p["util"]["ICESTORM_LC"], 5443)
        self.assertAlmostEqual(p["fmax"]["clk"], 18.40, places=2)


class TestBuildIce40(unittest.TestCase):
    def test_lc_ram_from_stat_no_fmax_when_pnr_absent(self):
        stat = emit_metrics.parse_yosys_stat(read("yosys_stat_ice40.txt"))
        doc = emit_metrics.build_ice40(stat, {"util": {}, "fmax": {}}, "icesugar", "abc", "j1")
        names = {m["name"]: m for m in doc["metrics"]}
        self.assertEqual(names["icesugar [j1]/LC"]["value"], 5443)
        self.assertEqual(names["icesugar [j1]/LC"]["dir"], "smaller")
        self.assertEqual(names["icesugar [j1]/RAM"]["value"], 9)
        self.assertNotIn("icesugar [j1]/Fmax", names)  # no route -> no Fmax
        self.assertEqual(doc["target"], "ice40-up5k")

    def test_fmax_added_when_pnr_routes(self):
        stat = emit_metrics.parse_yosys_stat(read("yosys_stat_ice40.txt"))
        doc = emit_metrics.build_ice40(stat, {"util": {}, "fmax": {"clk": 18.4}}, "icesugar", "abc", "j1")
        names = {m["name"]: m for m in doc["metrics"]}
        self.assertEqual(names["icesugar [j1]/Fmax"]["value"], 18.4)
        self.assertEqual(names["icesugar [j1]/Fmax"]["dir"], "bigger")

    def test_lc_from_pnr_util_when_stat_empty(self):
        """Real iCESugar production path: PLACEMENT failure. yosys stat lacks
        ICESTORM_* (stat={}); nextpnr prints the utilisation report (LC/RAM) but
        never reaches routing, so there is NO `Max frequency` line -> LC=5443,
        RAM=17 present, Fmax absent."""
        parsed_pnr = emit_metrics.parse_nextpnr_ice40_log(read("nextpnr_ice40_placefail.log"))
        doc = emit_metrics.build_ice40({}, parsed_pnr, "icesugar", "c", "j1")
        names = {m["name"]: m for m in doc["metrics"]}
        self.assertEqual(names["icesugar [j1]/LC"]["value"], 5443)
        self.assertEqual(names["icesugar [j1]/RAM"]["value"], 17)
        self.assertNotIn("icesugar [j1]/Fmax", names)  # no route -> no Fmax (the iCESugar case)

    def test_timing_fail_log_still_yields_fmax(self):
        """Distinct from placement failure: a design that PLACED but missed timing
        prints a `Max frequency` line, so Fmax is surfaced alongside the LC util."""
        parsed_pnr = emit_metrics.parse_nextpnr_ice40_log(read("nextpnr_ice40.log"))
        doc = emit_metrics.build_ice40({}, parsed_pnr, "icesugar", "c", "j1")
        names = {m["name"]: m for m in doc["metrics"]}
        self.assertIn("icesugar [j1]/Fmax", names)

    def test_pnr_util_lc_takes_precedence_over_stat(self):
        """nextpnr post-pack LC is authoritative; stat (pre-pack) value is overridden."""
        stat_with_different_lc = {"ICESTORM_LC": 9999, "ICESTORM_RAM": 5}
        pnr = {"util": {"ICESTORM_LC": 5443, "ICESTORM_RAM": 17}, "fmax": {}}
        doc = emit_metrics.build_ice40(stat_with_different_lc, pnr, "icesugar", "c", "j1")
        names = {m["name"]: m for m in doc["metrics"]}
        self.assertEqual(names["icesugar [j1]/LC"]["value"], 5443)  # nextpnr wins
        self.assertEqual(names["icesugar [j1]/RAM"]["value"], 17)   # nextpnr wins


class TestVariantName(unittest.TestCase):
    def test_ecp5_variant_suffix(self):
        # build_ecp5 must accept a variant and tag the name
        doc = emit_metrics.build_ecp5(
            {"util": {"TRELLIS_COMB": 5111}, "fmax": {"clk": 34.6}},
            "ulx3s", "abc", "j4-rom")
        names = [m["name"] for m in doc["metrics"]]
        self.assertIn("ulx3s [j4-rom]/LUT4", names)

    def test_ecp5_no_variant_is_bare(self):
        doc = emit_metrics.build_ecp5(
            {"util": {"TRELLIS_COMB": 5111}, "fmax": {}},
            "ulx3s", "abc", None)
        self.assertEqual(doc["metrics"][0]["name"], "ulx3s/LUT4")  # back-compat


if __name__ == "__main__":
    unittest.main()
