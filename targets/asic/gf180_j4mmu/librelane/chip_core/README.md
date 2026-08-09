# chip_core — flat whole-soc P&R (KianV-style)

`chip_core` is the entire J4+MMU soc hardened as **one flat block**: all logic
placed as std cells, only the vendor SRAM (`gf180mcu_fd_ip_sram`) left as
(auto-placed) macros. This is the KianV RISC-V topology — a single core P&R
that `chip_top` then wraps in the IO pad ring via the real LibreLane
Chip/Padring flow (no chip-level routing, which is what the earlier flat
`pad_ring` integration tripped over: DRT-0073 on the pad PAD terminals).

Contrast with `top/`, which hardens the same soc **hierarchically** (6 child
macros + glue). `chip_core` trades hierarchy for a single congestion domain so
the pad ring has one macro to abut.

## Regenerating `chip_core.v`

`chip_core.v` is generated: the hierarchical netlists (`top/soc.v` glue + the 6
child `*.v` bodies) flattened into one module, with the SRAM cells kept as
blackbox leaves. Use the script — do not hand-run yosys:

```sh
./gen_chip_core.sh                      # child netlists already on disk
REGEN_CHILDREN=1 ./gen_chip_core.sh     # rebuild them first (seconds each)
```

`REGEN_CHILDREN` drives `../run.sh macro=<child> OL_NETLIST_ONLY=1`, which
stops after `write_verilog` instead of hardening each child (a full `cpus`
harden is ~90 min and places a macro nobody uses any more).

Expected: one `soc` module, **17 SRAM instances** (7 icache + 10 dcache), the
rest std cells. The tri-state warnings during read are the inout pad signals
(`ice_spi_io`, gpio, sd_cmd) and are benign.

### `proc` before `flatten` is mandatory

The recipe in this README before 2026-08-09 omitted `proc`. Without it yosys
warns

```
Warning: Ignoring module soc because it contains processes (run 'proc' command first).
```

and `write_verilog` emits raw RTLIL processes — measured **4118 `initial`
blocks**. Nothing complains until the next `chip_top` run dies ~24 s in, at
Generate JSON Header:

```
ERROR: Failed to get a constant init value for \aic_irq_gen.sync: \_000465_
```

`gen_chip_core.sh` asserts 1 module / 17 SRAMs / **0** `initial` blocks so a
bad regeneration fails immediately and legibly instead of hours later.

## Hardening

```sh
OL_IMAGE=ghcr.io/librelane/librelane:3.0.5 ./run.sh macro=chip_core
```

9T / 3.3 V on LibreLane 3.0.5 (see `common.json`). The SRAM macros are declared
in `config.json` with lef/gds/lib but **no fixed instances** — LibreLane
auto-places them. `DIE_AREA` / `PL_TARGET_DENSITY_PCT` are tuned for the flat
soc; widen the die if placement legalization (DPL) or global route (GRT)
reports congestion.
