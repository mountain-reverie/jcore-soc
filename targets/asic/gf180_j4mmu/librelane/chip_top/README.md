# chip_top — full-chip IO pad ring (KianV-style Chip flow)

`chip_top` is the tapeout-shaped top level: the flat J4+MMU soc
(`../chip_core/chip_core.v`, module `soc`) wrapped in a **gf180mcu IO pad
ring** and hardened by LibreLane's built-in **`Chip` flow** (`meta.flow: Chip`
in `config.json`).

## Why the Chip flow (and not the flat pad_ring)

The earlier flat `pad_ring/` integration placed the pads as ordinary macros and
tried to detailed-route them, which 3.0.5's TritonRoute rejects: it computes
pin access even for the die-edge pad `PAD` terminals and hard-errors
`DRT-0073`. The `Chip` flow's `OpenROAD.PadRing` step instead assembles the
pads as a **ring** (abutted, powered by the pad-frame rails, not core-routed) —
exactly how KianV's RISC-V chip does it. That is the real fix.

## Structure

- `chip_top.sv` — AUTO-GENERATED wrapper. Instantiates `soc u_soc(...)` plus
  101 gf180 pads: 4 inputs (`in_c`), 81 outputs+bidir (`bi_t`), 16 power
  (`dvdd`/`dvss`). Single VDD/VSS domain (IO supply tied to core). The soc
  exposes split i/o/oe buses, so the wrapper merges each triple into one bidir
  pad: `qfl_io` (per-bit `qfl_io_oe`), `sd_dq` (shared `sd_dq_oe`), `gpio`
  (no oe → output-enabled). Generated from the `soc` port list; if that list
  changes, regenerate the pad instances and the `PAD_*`/`MACROS` in
  `config.json` to match (every `pad_<signal>` instance must appear in exactly
  one `PAD_{SOUTH,EAST,NORTH,WEST}` list).

The buffer-explosion lesson: `clk_sys`/`reset` enter through pads, so unlike
chip_core (top-level clock port) LibreLane does not auto-exclude them from the
pre-CTS resizer, which otherwise builds ~44k throwaway buffer trees and stalls
detailed placement. `RSZ_DONT_TOUCH_LIST: [clk_sys, reset]` scopes the
skip to the resizer only (CTS still builds the clock tree). The clock is
defined on `pad_clk_sys/Y` (the net that drives the flops), not the external
port.
- `config.json` — the Chip-flow config: `meta.flow: Chip`, `DIE_AREA`
  4800×4800, `CORE_AREA` inset 442 for the pad frame, `PAD_SOUTH/EAST/NORTH/
  WEST` (the 101 pad instance names, balanced ~25/side with power pads
  interspersed), the 17 SRAMs hand-placed in two edge columns under the
  `u_soc.` prefix, `PDN_CORE_RING` + `CONNECT_TO_PADS`, 9T/3.3V corners from
  `common.json`. `IGNORE_DISCONNECTED_MODULES` covers the pad cells (output
  pads leave `Y` unused).

## Hardening

```sh
OL_IMAGE=ghcr.io/librelane/librelane:3.0.5 ./run.sh macro=chip_top
# first-pass routing only (skip slow signoff):
OL_IMAGE=... OL_TO=OpenROAD.DetailedRouting ./run.sh macro=chip_top
```

CLOCK enters at `clk_sys_PAD` → net `pad_clk_sys/Y`. The clock period is 33 ns
(30 MHz, the KianV operating point); the soc is CPU-bound and does not yet meet
it — timing closure is the deferred Fmax-tuning axis, orthogonal to the pad
ring routing clean.
