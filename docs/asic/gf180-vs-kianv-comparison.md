# jcore J4+MMU flash SoC vs KianV — GF180MCU area comparison

**Target:** `targets/asic/gf180_j4mmu` (flash/XIP variant), GlobalFoundries 180 nm
(GF180MCU), via LibreLane 2.4.2 + the `gf180mcu_fd_sc_mcu7t5v0` standard cells and
`gf180mcu_fd_ip_sram` macros (ciel PDK pin `f6eeac7…`).
**Reference:** KianV RV32IMA-SV32 Linux/XV6 SoC (Hirosh Dabui), taped out on GF180MCU
via wafer.space — **20.1 mm² die at ~65 % core utilization**, cache-SRAM + MMU +
external-SDRAM architecture. (wafer.space/news/kianv-riscv-soc)

## Headline

At GF180, the jcore **J4 + MMU** flash SoC lands **at parity with KianV**. The
robust, routing-independent basis is **placed silicon** (cells + SRAM macros):
**12.75 mm²** with the boot scratchpad removed, versus KianV's ~13 mm² (both at
~65 % utilization) — i.e. jcore is now **~2 % under** KianV (it was ~5 % over
before the scratchpad removal). The two are genuine peers — both MMU/Linux-class,
both cache-SRAM-dominated, both external SDRAM + SPI-flash boot. The area is
**driven by the two 8 KB caches**, not core efficiency.

## Integrated top (this work)

Hierarchical black-box P&R: each sub-block hardened standalone to a LEF abstract, then
the top integrated with all 8 as black boxes + flattened interconnect (peak memory
~0.9 GB — the flat-flatten alternative OOM'd).

| Metric (LibreLane `design__*`) | Value |
|---|---|
| **Placed instance silicon (8 macros + glue)** | **12.75 mm²** |
| Core utilization | 66.6 % |
| Core area (placement) | 19.14 mm² |
| Die area (placement, core-only, no pad ring) | 19.52 mm² |

**Routing status (honesty):** placed silicon is the solid, routing-independent
number. The two integrated runs bracket the *routable* die:
- with the old 1.13 mm² boot_mem (13.91 mm² silicon), the top routed **fully
  through detailed routing + RCX** at **21.72 mm² die / 65.3 % util** (it only
  failed at the cosmetic final GDS merge, which needs the child-macro GDS);
- with the ROM-only boot_mem (12.75 mm² silicon), a tighter 2-row **19.52 mm² /
  66.6 %** floorplan reached placement but **hit global-routing congestion
  (GRT-0118)** — it is not a fully-routed die.

So the **routable** core-only die with the smaller boot_mem is **~20 mm²**
(≈ 12.75 mm² silicon / ~0.64 routable util, between the 19.52 congested placement
and the 21.72 fully-routed prior run) — a ~1.5–2 mm² improvement from the
scratchpad removal, but the exact routed die at a congestion-clean util is a
follow-up. The **placed-silicon** comparison to KianV does not depend on this.

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

The top-level integrated P&R was **re-run** with the smaller boot_mem (new
floorplan: the old 3-row macro grid collapsed to 2 rows since row1
[icache|dcache] already fixes the die width independent of boot_mem's size).
Measured: **placed silicon 13.91 → 12.75 mm²** (a 1.16 mm² saving, matching the
1.13 → 0.02 mm² boot_mem delta). The tighter 19.52 mm² / 66.6 % floorplan reached
placement but hit global-routing congestion (see "Routing status" above), so the
routable die is ~20 mm² rather than a clean 19.52 — placed silicon is the honest
figure; the exact routed die at a congestion-clean util is a follow-up.

## Comparison

| | jcore J4+MMU flash | KianV |
|---|---|---|
| Placed silicon (cells + SRAM) | **12.75 mm²** (measured, post-scratchpad-removal) | ~13 mm² |
| Core / die basis | ~20 mm² routable core-only (placement 19.52 mm², routing-congested) | 20.1 mm² padded die |
| ISA / class | SH-2-compatible J4, SH-4-class MMU | RV32IMA + SV32 |
| Linux-capable | yes (MMU in RTL) | yes (uLinux/XV6) |
| On-chip cache SRAM | 2×8 KB (vendor macros) | yes ("cache SRAM around the core") |
| External DRAM | SDRAM controller | 32 MiB SDRAM controller |
| Flash boot | QSPI XIP (PR #98) | SPI-flash XIP |

**Verdict:** the J4+MMU flash SoC **fits the same ~20 mm² GF180 die class as KianV, at
parity — now dead-even, ~2 % under KianV on placed silicon.** Both are SRAM-dominated;
the lever for either is cache size. This corrects an earlier naive impression of large
headroom that came from summing per-block die boxes (double-counting whitespace)
against KianV's full die — the integrated 12.75 mm² placed-silicon figure (measured,
post-scratchpad-removal) is the honest, like-for-like number.

## Basis / caveats (honesty)

- **Core-only** (no pad ring). A `gf180mcu_fd_io` pad ring (B-scope) would push the
  jcore *die* above core, further past KianV's 20.1 mm². KianV's number is a full
  padded die, so jcore is if anything understated here.
- **Density sensitivity:** the caches were hardened loose (internal util ~15 %), so
  their LEF footprints (5.06 mm² each) carry macro-internal whitespace; the top's
  65.3 % util sits on top of that. A tighter cache hardening would reduce the die. The
  placed-silicon parity holds regardless.
- **Routing:** the scratchpad-free 19.52 mm² floorplan hit global-routing
  congestion (GRT-0118) at 66.6 % util — routable die is ~20 mm² (the prior
  13.91 mm²-silicon top routed fully at 21.72 mm²/65 %). Placed silicon (12.75
  mm²) is routing-independent and is the number the KianV comparison rests on.
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
