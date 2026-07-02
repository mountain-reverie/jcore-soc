window.BENCHMARK_DATA = {
  "entries": {
    "synth-size": [
      { "commit": { "id": "legacy" }, "date": 1715817600000, "benches": [
        { "name": "ecp5-lfe5u-85f · ulx3s/LUT4", "unit": "cells", "value": 5090, "extra": "ulx3s" }
      ] },
      { "commit": { "id": "demo" }, "date": 1718409600000, "benches": [
        { "name": "ecp5-lfe5u-85f · ulx3s [j2-direct]/LUT4", "unit": "cells", "value": 5111, "extra": "ulx3s" },
        { "name": "ecp5-lfe5u-85f · ulx3s [j4-rom]/LUT4", "unit": "cells", "value": 5340, "extra": "ulx3s" },
        { "name": "ice40-up5k · icesugar [j1]/LC", "unit": "cells", "value": 5218, "extra": "icesugar" },
        { "name": "ice40-up5k · icesugar [j1]/RAM", "unit": "cells", "value": 17, "extra": "icesugar" }
      ] }
    ],
    "synth-speed": [
      { "commit": { "id": "legacy" }, "date": 1715817600000, "benches": [
        { "name": "ecp5-lfe5u-85f · ulx3s/Fmax", "unit": "MHz", "value": 33.10, "extra": "ulx3s" }
      ] },
      { "commit": { "id": "demo" }, "date": 1718409600000, "benches": [
        { "name": "ecp5-lfe5u-85f · ulx3s [j2-direct]/Fmax", "unit": "MHz", "value": 34.64, "extra": "ulx3s" },
        { "name": "ice40-up5k · icesugar [j1]/Fmax", "unit": "MHz", "value": 13.76, "extra": "icesugar" }
      ] }
    ]
  }
};
