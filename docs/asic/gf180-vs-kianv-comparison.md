# jcore J4+MMU flash SoC vs KianV — GF180MCU area comparison

**Target:** `targets/asic/gf180_j4mmu` (flash/XIP variant), GlobalFoundries 180 nm
(GF180MCU), via LibreLane 2.4.2 + the `gf180mcu_fd_sc_mcu7t5v0` standard cells and
`gf180mcu_fd_ip_sram` macros (ciel PDK pin `f6eeac7…`).
**Reference:** KianV RV32IMA-SV32 Linux/XV6 SoC (Hirosh Dabui), taped out on GF180MCU
via wafer.space — **20.1 mm² die at ~65 % core utilization**, cache-SRAM + MMU +
external-SDRAM architecture. (wafer.space/news/kianv-riscv-soc)

## Headline

At GF180, the jcore **J4 + MMU** flash SoC lands **at parity with KianV — no
meaningful headroom.** Its routed integrated core is **21.3 mm²** with **13.9 mm² of
placed silicon** (macros + logic) at **65 % utilization**, versus KianV's 20.1 mm²
die (~13 mm² placed silicon at ~65 %). The two are genuine peers — both MMU/Linux-
class, both cache-SRAM-dominated, both using an external SDRAM controller + SPI-flash
boot. The area is **driven by the two 8 KB caches**, not core efficiency.

## Routed integrated top (this work)

Hierarchical black-box P&R: each sub-block hardened standalone to a LEF abstract, then
the top placed+routed with all 8 as black boxes + flattened interconnect (kept peak
memory ~0.9 GB — the flat-flatten alternative OOM'd).

| Metric (LibreLane `design__*`) | Value |
|---|---|
| Die area (core-only, no pad ring) | **21.72 mm²** |
| Core area | 21.32 mm² |
| Placed instance silicon (8 macros + glue) | **13.91 mm²** |
| Core utilization | **65.3 %** |

## Per-block routed areas (GF180, standalone)

| Block | Routed die (mm²) | Notes |
|---|---|---|
| j4_core (CPU + MMU/TLB) | 1.22 | std-cell logic, no macro |
| icache (8 KB, vendor SRAM) | 5.06 | 2×`sram256x8` tag + 16×`sram512x8` data |
| dcache (8 KB, vendor SRAM) | 5.06 | 4×`sram256x8` tag + 16×`sram512x8` data |
| boot_mem (logic-ROM vector, ROM-only, no scratchpad) | 0.02 | 0 SRAM macros; pure logic ROM |
| sdram_ctrl | 0.05 | full RTL→GDS clean (DRC/LVS) |
| devices (UART/SPI/GPIO/AIC) | 0.41 | logic |
| qspi_flash_ctrl | 0.56 | logic |
| mem_region_mux | 0.09 | logic |

## Boot-memory refinement: scratchpad SRAM eliminated

The 2 KB writable boot-time stack SRAM (4×`sram512x8` vendor macros) has been
**removed entirely**. `components/sdram/sdram_ctrl.vhd` self-initialises in
hardware (its FSM runs the full SDRAM init sequence from reset and only
serves accesses once idle), so the reset vector's SP now points directly
into SDRAM instead of an on-chip scratchpad — proven end-to-end in cosim
(`targets/asic/gf180_j4mmu/sim/xip_sim.sh`: reset → ROM vector (PC=flash,
SP=SDRAM) → flash XIP execute → a real SH-2 stack push lands in SDRAM at
0x10000ffc, `XIP_SIG_OK`). `boot_mem` is now **pure read-only ROM** — no
vendor SRAM macro at all.

Re-hardened standalone (LibreLane, `run.sh macro=boot_mem`, util-driven
floorplan sized only for the block's 155 flattened-record IO pins, no
`MACROS` entry): **die 0.0196 mm² (140×140 µm), instance area 0.00272 mm²,
`design__instance__area__macros = 0`** — confirmed zero `sram512x8`
placements in both the routed DEF and the netlist. Routed die area
**1.13 mm² → 0.02 mm²**, a measured **~1.11 mm² saving** on this block
standalone.

The top-level integrated P&R (13.91 mm² placed-silicon / 21.72 mm² die
figures below) has **not** been re-run with the smaller boot_mem yet — that
is a separate, larger job (flagged as follow-up, not required for this
refinement). Naively propagating the standalone saving gives an **estimated**
new placed-silicon total of **13.91 − 1.11 ≈ 12.80 mm²** (≈ 13.07 mm² if
using the coordinator's original ~0.84 mm² planning estimate instead of the
measured 1.11 mm² — the measured number is used here as the honest figure,
and both remain estimates pending an actual top re-run).

## Comparison

| | jcore J4+MMU flash | KianV |
|---|---|---|
| Placed silicon (cells + SRAM) | **13.9 mm²** (≈12.8 mm² est. post-scratchpad-removal, top not re-run) | ~13 mm² |
| Core @ ~65 % util | **21.3 mm²** (top not re-run since boot_mem shrink) | 20.1 mm² (die) |
| ISA / class | SH-2-compatible J4, SH-4-class MMU | RV32IMA + SV32 |
| Linux-capable | yes (MMU in RTL) | yes (uLinux/XV6) |
| On-chip cache SRAM | 2×8 KB (vendor macros) | yes ("cache SRAM around the core") |
| External DRAM | SDRAM controller | 32 MiB SDRAM controller |
| Flash boot | QSPI XIP (PR #98) | SPI-flash XIP |

**Verdict:** the J4+MMU flash SoC **fits the same ~20 mm² GF180 die class as KianV, at
parity (≈5 % larger core), not with headroom.** Both are SRAM-dominated; the lever for
either is cache size. This corrects an earlier naive impression of large headroom that
came from summing per-block die boxes (double-counting whitespace) against KianV's full
die — the integrated 13.9 mm² placed-silicon figure is the honest, like-for-like number.

## Basis / caveats (honesty)

- **Core-only** (no pad ring). A `gf180mcu_fd_io` pad ring (B-scope) would push the
  jcore *die* above core, further past KianV's 20.1 mm². KianV's number is a full
  padded die, so jcore is if anything understated here.
- **Density sensitivity:** the caches were hardened loose (internal util ~15 %), so
  their LEF footprints (5.06 mm² each) carry macro-internal whitespace; the top's
  65.3 % util sits on top of that. A tighter cache hardening would reduce the die. The
  placed-silicon parity holds regardless.
- **Timing** is best-effort/non-gating (ideal 40 ns clock); this is an **area** proxy,
  not a signed-off tapeout (no DRC/LVS/RCX signoff at the top; the final GDS merge is
  a documented finishing step needing the child macros' GDS).
- The design is **functionally validated**: the XIP cosim boots the *real* end-to-end
  path (reset → logic-ROM vector, PC=flash/SP=SDRAM → flash XIP → SDRAM stack push,
  no on-chip scratchpad — `sim/xip_sim.sh`, `XIP_SIG_OK`).

## Design corrections made for an honest comparison

- **Boot memory (silicon-correct, then scratchpad eliminated):** replaced a 16 KB
  inferred-flop boot RAM (~7 mm², and *incorrect in silicon* — an SRAM macro powers up
  undefined and can't hold a reset vector) with a synthesized constant **logic-ROM
  vector** + a 2 KB stack SRAM (1.13 mm², ~6× smaller, silicon-correct). Then, once it
  was confirmed that `sdram_ctrl` self-initialises in hardware, the writable stack
  scratchpad was **removed entirely** — the boot-time stack lives in SDRAM instead,
  proven in cosim — leaving **boot_mem as pure read-only ROM, 0.02 mm², no SRAM macro**.
- **Vendor SRAM actually wired in:** the ASIC target had been *measuring* vendor SRAM
  standalone while the wired SoC still used inferred-flop cache/boot RAM. The flash
  variant is now genuinely bound to the vendor-SRAM cache + boot architectures, and
  re-validated to boot.
