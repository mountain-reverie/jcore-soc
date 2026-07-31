# GF180 flat chip integration with a KianV-style IO pad ring (`macro=pad_ring`)

This is the **full-chip** GF180 flow for the flash-variant J4+MMU SoC: the
`soc` core and a peripheral pad ring of `gf180mcu_fd_io` cells routed together
in **one flat routing domain**, producing a manufacturable die outline.

## Result

| integration | die | pad ring | route | DRC |
|---|---|---|---|---|
| soc-as-macro (soc hardened to LEF, then wrapped in a pad ring) | 5080×5080 = **25.8 mm²** | yes | macro-boundary | 0 |
| **flat (soc children black-boxed, glue + pads in one domain)** | **4120×4242 = 17.5 mm²** | yes | flat, direct-OpenROAD | **0** |
| KianV reference (same PDK, comparable IO count) | — = 20.1 mm² | yes | — | — |

Flattening removes the hard soc/pad-ring macro boundary, so the router places
and connects the pad drivers directly against the core logic. That recovers
~8 mm² and brings the whole chip **below the KianV reference** — see
`docs/asic/gf180-vs-kianv-comparison.md`.

## Why flat, and why direct OpenROAD

- **Flat** — a soc-as-macro boundary forces every pad-to-core net through the
  macro's pin ring and reserves keep-out around it. Black-boxing the 6 soc
  children (`cpus`, the 2 cache adapters, `sdram_ctrl`, `qspi_flash_ctrl`,
  `devices`) but keeping the top interconnect glue + the pads as real cells in
  one domain lets the placer interleave them and the die shrinks.
- **Direct OpenROAD route** — the flat design is congested enough that GRT
  emits GRT-0118, which LibreLane's `grt` step treats as a hard error.
  `route.tcl` runs `global_route -allow_congestion` (GRT resolves the overflow
  in detailed route → 0 DRC) and marks the `*_PAD` chip terminals + power/ground
  nets `$setSpecial` so they are not signal-routed (they are strapped by pad
  abutment).

## The pad ring

58 `bi_t` bidirectional signal pads + 16 `dvdd`/`dvss` power pads, KianV-style,
spread **evenly around the perimeter** (S18 / E19 / N18 / W19). Signal set:
clk_sys, reset, uart0, spi2 (+cs), qspi flash (cs/sck/io[4]), SDRAM
(cmd/addr[13]/ba[2]/dqm[2]/dq[16]), gpio[5]. `bi_t` wiring: `A`=core→pad,
`Y`=pad→core, `OE`/`IE` per direction; the control pins (CS/SL/PU/PD/PDRV*) tie
to power. Even distribution matters: an early build clustered all SDRAM pads on
the south edge and that congestion was the source of the last routing DRCs.

## Flow

Everything except the two generators' outputs is committed. The netlist and
config are **generated** because they bind the ciel PDK IO-cell LEFs by absolute
path (machine-specific).

```bash
cd targets/asic/gf180_j4mmu/librelane/pad_ring

# 1. generate the netlist (pad_ring.v + pad_cells_bb.v IO stubs) and the config
python3 gen_netlist.py          # 58 signal + 16 power pads
python3 gen_config.py [chan_um] # flat die, pads spread evenly (default 150um channel)

# 2. harden through placement + CTS (LibreLane; stops at OpenROAD.CTS)
cd .. && ./run.sh macro=pad_ring

# 3. finish the route with direct OpenROAD on the post-CTS odb (see route.tcl
#    header for the read_* preamble); marks PAD/power nets special, allows
#    congestion. Writes routed_flat.def, 0 DRC.
#    -> pad_ring/runs/routed_flat.def
```

`gen_config.py` consumes `../top/soc.v` + `../top/soc_macros_bb.v` (the top's
committed hierarchy netlist — see `../top/README.md`) as the flattened soc, so
the pad ring always tracks the same SoC RTL as the `macro=top` flow.

## Committed inputs vs generated outputs

- **Committed:** `gen_netlist.py`, `gen_config.py`, `route.tcl`, this README.
- **Generated (git-ignored):** `pad_ring.v`, `pad_cells_bb.v`, `config.json`,
  `runs/`. Reproduce them with the two generators above.
