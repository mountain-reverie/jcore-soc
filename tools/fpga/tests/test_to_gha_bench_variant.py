import json
import os
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, os.path.dirname(HERE))
import to_gha_bench  # noqa: E402


class TestVariantSeries(unittest.TestCase):
    def _write(self, doc):
        fd, p = tempfile.mkstemp(suffix=".json")
        os.close(fd)
        with open(p, "w") as f:
            json.dump(doc, f)
        return p

    def test_variant_suffix_preserved_and_distinct(self):
        a = self._write({"target": "ecp5-lfe5u-85f", "board": "ulx3s",
            "metrics": [{"name": "ulx3s [j2-direct]/LUT4", "unit": "cells", "value": 5111, "dir": "smaller"}]})
        b = self._write({"target": "ecp5-lfe5u-85f", "board": "ulx3s",
            "metrics": [{"name": "ulx3s [j4-rom]/LUT4", "unit": "cells", "value": 5300, "dir": "smaller"}]})
        ice = self._write({"target": "ice40-up5k", "board": "icesugar",
            "metrics": [{"name": "icesugar [j1]/LC", "unit": "cells", "value": 5443, "dir": "smaller"}]})
        size, speed = to_gha_bench.convert([a, b, ice])
        names = sorted(e["name"] for e in size)
        self.assertEqual(names, [
            "ecp5-lfe5u-85f · ulx3s [j2-direct]/LUT4",
            "ecp5-lfe5u-85f · ulx3s [j4-rom]/LUT4",
            "ice40-up5k · icesugar [j1]/LC",
        ])
        for p in (a, b, ice):
            os.unlink(p)


if __name__ == "__main__":
    unittest.main()
