# jcore J4+MMU flash SoC vs KianV — GF180MCU area comparison

**Target:** `targets/asic/gf180_j4mmu` (flash/XIP variant), GlobalFoundries 180 nm
(GF180MCU), via LibreLane 2.4.2 + the `gf180mcu_fd_sc_mcu7t5v0` standard cells and
`gf180mcu_fd_ip_sram` macros (ciel PDK pin `f6eeac7…`).
**Reference:** KianV RV32IMA-SV32 Linux/XV6 SoC (Hirosh Dabui), taped out on GF180MCU
via wafer.space — **20.1 mm² die at ~65 % core utilization**, cache-SRAM + MMU +
external-SDRAM architecture. (wafer.space/news/kianv-riscv-soc)

## Headline

At GF180, the jcore **J4 + MMU** flash SoC lands **at parity with KianV**. The
robust, fair basis is **placed silicon** (cells + SRAM macros): **12.80 mm²**
with the boot scratchpad removed, versus KianV's ~13 mm² (both at ~61–65 %
utilization) — i.e. jcore is now **~2 % under** KianV (it was ~5 % over before
the scratchpad removal). This is now backed by a **fully-routed** integrated
top (detailed routing + RCX clean, no GRT-0118 congestion) at **21.20 mm² die
/ 61.5 % util** — not a placement estimate. The two are genuine peers — both
MMU/Linux-class, both cache-SRAM-dominated, both external SDRAM + SPI-flash
boot. The area is **driven by the two 8 KB caches**, not core efficiency.

## Integrated top (this work)

Hierarchical black-box P&R: each sub-block hardened standalone to a LEF abstract, then
the top integrated with all 8 as black boxes + flattened interconnect (peak memory
~0.9 GB — the flat-flatten alternative OOM'd).

| Metric (LibreLane `design__*`, from `48-openroad-rcx/or_metrics_out.json`) | Value |
|---|---|
| **Placed instance silicon (8 macros + glue)** | **12.80 mm²** |
| Core utilization | 61.5 % |
| Core area (fully-routed) | 20.81 mm² |
| Die area (fully-routed, core-only, no pad ring) | 21.20 mm² |

**Routing status (honesty):** this is now a **measured, fully-routed** die —
detailed routing + RCX completed cleanly with no GRT-0118 congestion — not a
placement estimate. Two floorplans were tried with the ROM-only (0.02 mm²)
boot_mem:
- a tight 2-row **19.52 mm² / 66.6 %** floorplan reached placement/CTS but
  **hit global-routing congestion (GRT-0118)** — it does not route;
- widening the die to **21.20 mm² / 61.5 %** (doubling the row1→row2 channel
  and the row2 inter-macro gaps) **routes clean through detailed routing +
  RCX** — it only stops at the cosmetic final GDS merge (`Magic.StreamOut`),
  which needs the child-macros' GDS, same as every other run here.

So the cache-dominated design needs roughly **~61 % util to route**; a tighter
~65–67 % floorplan congests. The **placed-silicon** comparison to KianV does
not depend on routing headroom either way.

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

## 2 KB cache variant (this work): measured, then composed

The area is **cache-dominated**, so the honest lever is cache size. Both caches were
re-hardened standalone at **2 KB** (24-bit tag, `CACHE_INDEX_BITS=6`) through the same
LibreLane detailed-routing + RCX flow (`run.sh macro=icache_2k` / `macro=dcache_2k`,
stop at `Magic.WriteLEF`), binding the widened tag to 3×`sram64x8` and the data to
4×`sram512x8` vendor macros (vs the 8 KB build's 2–4×`sram256x8` tag + 16×`sram512x8`
data). Both routed clean.

| 2 KB cache (measured, routed) | Placed silicon (cells + SRAM) | SRAM macros | Routed die | Util |
|---|---|---|---|---|
| icache_2k | **1.32 mm²** | 1.14 mm² (7 macros) | 4.32 mm² | 31.3 % |
| dcache_2k | **1.79 mm²** | 1.44 mm² (10 macros) | 8.12 mm² | 22.4 % |
| **both 2 KB caches** | **3.11 mm²** | 2.58 mm² | — | — |

**Placed silicon is the comparable basis, not the die box.** The two routed dies differ
(4.32 vs 8.12 mm²) only because `dcache_2k` was hardened on a deliberately loose
floorplan (util 22 %) to clear an OpenROAD CTS clock-buffer legalization failure
(`DPL-0036` on `clkbuf_3_*`: the level-3 clock buffers globally place over the clustered
macro block and can't legalize nearby on a tight die — enlarging the die spreads them
into open sites). That extra die area is routing/legalization whitespace, not silicon.

**Composed 2 KB top (placed silicon).** Swapping the two 8 KB caches out of the measured
12.80 mm² integrated top for the 2 KB ones:

```
top(2 KB) = top(8 KB) − caches(8 KB) + caches(2 KB)
          = 12.80  − (3.83 + 4.19)  + (1.32 + 1.79)
          = 12.80  −  8.03          +  3.11
          = 7.89 mm² placed silicon
```

a **−4.91 mm² (−38 %)** reduction in placed silicon vs the 8 KB top. At the 8 KB top's
measured **61.5 % routing utilization**, that composes to an implied **core die of
≈ 7.89 / 0.615 ≈ 12.8 mm²** (core-only, no pad ring — same basis caveat as the 8 KB
top). So the 2 KB variant lands **~39 % under KianV on placed silicon** (7.89 vs
~13 mm²) and its estimated core die (~12.8 mm²) sits **~36 % under KianV's 20.1 mm²
padded die** — even before accounting for the pad ring KianV's number includes and
jcore's does not. The composed die is an **estimate** (the two 2 KB caches are measured
+ routed; the top was not re-integrated with them), whereas the 12.80 mm² 8 KB
placed-silicon figure is a measured integrated result.

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
Measured: **placed silicon 13.91 → 12.80 mm²** (a 1.11 mm² saving, matching the
1.13 → 0.02 mm² boot_mem delta almost exactly). The **routed die**, however,
only moved **21.72 → 21.20 mm²** (a 0.52 mm² saving) — smaller than the silicon
saving. The reason: the tight 19.52 mm² / 66.6 % floorplan that the silicon
saving alone would suggest **congests** (GRT-0118); routing this cache-heavy
design cleanly needs ~61 % util, so most of the row-height reclaimed by
dropping the scratchpad had to be given back as routing channel headroom
(wider row1↔row2 gap, wider row2 inter-macro gaps) rather than shrinking the
die 1:1 with the silicon. **Placed silicon is the honest, routing-independent
saving (−1.11 mm²); the routed die saving is smaller (−0.52 mm²) because
routing overhead eats into it.**

## Comparison

| | jcore J4+MMU flash | KianV |
|---|---|---|
| Placed silicon (cells + SRAM) | **12.80 mm²** (measured, post-scratchpad-removal) | ~13 mm² |
| Core / die basis | 21.20 mm² core-only, **fully-routed** (RCX-clean, 61.5 % util, no pad ring) | 20.1 mm² padded die |
| ISA / class | SH-2-compatible J4, SH-4-class MMU | RV32IMA + SV32 |
| Linux-capable | yes (MMU in RTL) | yes (uLinux/XV6) |
| On-chip cache SRAM | 2×8 KB (vendor macros) | yes ("cache SRAM around the core") |
| External DRAM | SDRAM controller | 32 MiB SDRAM controller |
| Flash boot | QSPI XIP (PR #98) | SPI-flash XIP |

**Verdict:** the J4+MMU flash SoC **fits the same ~20 mm² GF180 die class as KianV, at
parity — now dead-even, ~2 % under KianV on placed silicon.** Both are SRAM-dominated;
the lever for either is cache size. This corrects an earlier naive impression of large
headroom that came from summing per-block die boxes (double-counting whitespace)
against KianV's full die — the integrated **12.80 mm² placed-silicon** figure
(measured, post-scratchpad-removal) is the honest, like-for-like number. The
**21.20 mm² fully-routed core-only die** is a real, measured result (detailed
routing + RCX clean), but it is core-only/no-pad-ring against KianV's padded
20.1 mm², so it is not the clean apples-to-apples basis — placed silicon is.
The caveats below (core-only, timing best-effort, GDS-merge finishing step,
routing-congestion sensitivity of this cache-dominated design) still apply.

## Basis / caveats (honesty)

- **Core-only** (no pad ring). A `gf180mcu_fd_io` pad ring (B-scope) would push the
  jcore *die* above core, further past KianV's 20.1 mm². KianV's number is a full
  padded die, so jcore is if anything understated here.
- **Density sensitivity:** the caches were hardened loose (internal util ~15 %), so
  their LEF footprints (5.06 mm² each) carry macro-internal whitespace; the top's
  65.3 % util sits on top of that. A tighter cache hardening would reduce the die. The
  placed-silicon parity holds regardless.
- **Routing:** the scratchpad-free design routes cleanly (detailed routing +
  RCX, no GRT-0118) at a widened **21.20 mm² / 61.5 %** floorplan, but a
  tighter **19.52 mm² / 66.6 %** floorplan of the same netlist **congests**
  (GRT-0118) and does not route. This cache-heavy design needs meaningfully
  more routing headroom than a first-cut ~65–67 % util floorplan gives it.
  Placed silicon (12.80 mm²) doesn't depend on this either way and is the
  number the KianV comparison rests on.
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
