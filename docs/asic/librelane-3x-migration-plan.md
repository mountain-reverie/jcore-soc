# GF180 flow migration: LibreLane 2.4.2 → 3.x

**Goal:** move the `targets/asic/gf180_j4mmu` flow from LibreLane 2.4.2 to
LibreLane 3.x, so the flow rides a modern, maintained base — and, critically,
unblocks the **9T @ 3.3 V KianV-matched** operating point (impossible on 2.4.2:
its explicit-corner path fails `DFFLEGALIZE`; 3.0.5 clears it — proven). Fmax
optimization work comes *after* this migration.

## Why this, why now

The 9T/3.3 V bring-up hit a hard wall on 2.4.2:
- 2.4.2 fails `DFFLEGALIZE: $_DFFE_ cannot be legalized` on the explicit 3.3 V
  corner path. **3.0.5 synthesizes `cpus` cleanly at 9T/3.3 V** (validated).
- KianV taped out on `github:librelane/librelane` branch `leo/gf180mcu` — an
  even-newer base than any 2.x release — with a modern `LIB`/`STA_CORNERS`/
  `DEFAULT_CORNER` config and **no compat overlay**.

Our 2.4.2 flow leans on a bespoke `pdk_overlay/.../config.tcl` that is entirely
a **2.4.x compat shim** (`CELL_LIBS` dict, `LIB_SYNTH/SLOWEST/FASTEST`,
`migrate_old_config` KeyError workarounds, `FP_PDN_*` aliases). On 3.x that shim
is the *blocker* (3.0.5 can't parse the `CELL_LIBS` dict), not the enabler.

## Key finding that shrinks the scope

**LibreLane 3.x supports gf180mcu natively** (fully supported PDK on main). So
the migration is expected to *delete* most of the compat overlay rather than
rewrite it: 3.x + a properly-fetched gf180mcu PDK + a KianV-style design config
should need little-to-no `config.tcl` shim. That is the central hypothesis
Phase 1 must confirm.

## Decisions (locked 2026-07-31)

- **PDK variant: gf180mcuD** (3.x-native / shuttle-recommended; KianV uses D).
- **Execution: Phase 1 first** (one-macro de-risk on 3.0.5).

## Strategic decisions (resolve before/at Phase 1)

1. **PDK variant + fetch.** We use ciel pin `f6eeac7` (gf180mcuC). 3.x docs note
   future shuttles need **gf180mcuD**; C is what GFMPW0 used. Decide: stay on
   ciel-C for continuity, or move to the 3.x-native PDK fetch (volare/ciel pin
   that 3.x expects, possibly D). KianV uses D via nix-eda.
2. **Drop vs keep the overlay.** Hypothesis: drop it. Confirm 3.x handles the
   ciel PDK's own librelane config natively; keep only genuinely-needed
   design-level settings (in `common.json`, KianV-style).
3. **Config format.** Adopt KianV's `LIB` corner-map + `STA_CORNERS` +
   `DEFAULT_CORNER` (3.x-native). SRAM libs attach per-macro via `MACROS`, not
   the global LIB (already learned: mixing them breaks dfflibmap).
4. **Docker image.** Pin `OL_IMAGE=ghcr.io/librelane/librelane:3.0.5` (or newer
   stable 3.x) in `run.sh` + the die-area CI.

## Phased plan

**Phase 0 — Scoping (no P&R).** Confirm how 3.x sets up gf180mcu: does it use
our ciel PDK as-is, or a different fetch? Inventory 2.4.2→3.x variable/step
renames touching our configs (`OL_TO`/`OL_SKIP` step ids, `LIB` vs `CELL_LIBS`,
PDN vars). Deliverable: the target `common.json` + overlay decision.

**Phase 1 — One-macro bring-up (de-risk).** Get **one macro end-to-end on 3.0.5
at 9T/3.3 V** (synth → floorplan → PDN → CTS → route → WriteLEF), with the
minimal/no overlay. `cpus` (Fmax-relevant, no SRAM) + one cache (`icache_2k`,
exercises SRAM corners) cover both cell classes. Deliverable: a working config
template + the definitive overlay-or-not answer.

**Phase 2 — Roll out to all macros.** Apply the template to all 8 macros;
re-tune floorplans for 9T's ~30 %-bigger cells (7T-tuned `DIE_AREA`/util will
need bumps). Deliverable: all macros hardened at 9T/3.3 V.

**Phase 3 — Pad ring + top.** Port the flat pad-ring flow (`run.sh` pad_ring
branch, `gen_netlist.py`/`gen_config.py`, `route.tcl`, `finish_route.sh`) to
3.x — step names, the direct-OpenROAD route, IO-cell handling. Deliverable: the
full padded die at 9T/3.3 V.

**Phase 4 — CI.** Update the `gf180-die-area` job (and any synth-metrics use) to
the 3.0.5 image. Re-validate headless (the same shakeout as the 2.4.2 CI work).

**Phase 5 — Re-measure + hand off to Fmax.** Re-measure area (the true
KianV-matched like-for-like) + the padded die. This is the clean baseline the
**Fmax optimization** work then builds on (clock sweeps, the register-cone /
debug-output levers, 9T already in place).

## What's already done (carry-in)

- **Proven:** 3.0.5 synthesizes `cpus` at 9T/3.3 V (the fundamental unblock).
- **9T overlay fixes** (needed regardless): the `config.tcl` `SYNTH_DRIVING_
  CELL_PIN` shim + `sc9` placement site (aligned to the working 7T config).
- **Config learnings:** SRAM libs must NOT be in the global `LIB` (breaks
  dfflibmap) — attach per-macro; `DEFAULT_CORNER` is required; KianV's exact
  `LIB`/`STA_CORNERS` map (mirrored in this branch's `common.json`).
- **Fmax groundwork** (for Phase 5): SoC is CPU-bound; `cpus` core reg-to-reg
  ~42 MHz @ 5 V (7T), limited by one high-fanout register cone feeding the
  34-bit `debug_o_d` output; memory (SDRAM/flash/cache) is 3–4× faster.

## Risk / effort

Real major-version migration; expect step-name and config-schema shakeout
(like the 2.4.2 CI bring-up). Phase 1 is the de-risker — if one macro goes
clean end-to-end on 3.0.5, the rest is mechanical rollout. If 3.x's PDK/config
model diverges more than expected, re-scope after Phase 1.

## Phase 1 findings (2026-07-31)

**The crux is PDK config provisioning, not the design config.** Confirmed:
- 3.0.5 uses an EXTERNAL PDK via `PDK_ROOT` (no volare, no bundled gf180mcu
  config) — same as 2.4.2. It reads the PDK's own `libs.tech/librelane/config.tcl`.
- Our ciel pin `f6eeac7`'s config.tcl (both gf180mcuC and D — byte-identical)
  is the **2.4.x-era `CELL_LIBS`/5v00 format** that 3.0.5 cannot parse
  ("cannot read file *_tt_025C_...").
- KianV avoids this by getting the PDK from **nix-eda**, which ships a
  **3.x-native** librelane PDK config (modern `LIB` dict) — not ciel's config.tcl.

**Two paths to a 3.x-native PDK config (pick in Phase 1):**
  A. **Newer PDK pin** — ciel has newer gf180mcu pins than `f6eeac7`
     (d658698, f3c505b, ...). If a newer pin ships a modern (3.x) config.tcl,
     we drop our overlay and use it directly. TESTING THIS FIRST (cheapest).
  B. **Overlay rewrite** — replace our 2.4.x compat `config.tcl` with a clean
     3.x-native gf180mcu PDK config (modern `LIB` dict, no `CELL_LIBS`/
     `migrate_old_config` shims). Bounded, fully in our control, but more work.

**Path A RULED OUT:** newer pin d658698 ships the identical 2.4.x CELL_LIBS config.tcl (no yaml). All ciel gf180mcu pins are 2.4.x-era. -> **Path B** (strip corner defs from overlay; define corners via common.json LIB, KianV-style).

## Phase 1 RESULT: 3.x-native config template WORKS ✅ (2026-07-31)

`cpus` synthesizes cleanly at 9T/3.3V on **LibreLane 3.0.5 + gf180mcuD** with
the config below (reads the correct `9t5v0__tt_025C_3v30.lib`, past DFFLEGALIZE,
into ABC — no corner/CELL_LIBS/TclError). **This is the migration template:**

1. **Docker:** `OL_IMAGE=ghcr.io/librelane/librelane:3.0.5`.
2. **PDK:** `gf180mcuD` (ciel pin f6eeac7 already ships it).
3. **Overlay:** STRIP the corner defs from `pdk_overlay/.../config.tcl` — remove
   the `CELL_LIBS` dict, `STA_CORNERS`, `DEFAULT_CORNER`, `LIB_SYNTH/FASTEST/
   SLOWEST` blocks (the 2.4.x compat shim that 3.x can't parse). Keep the
   physical/tech config (tech+cell LEFs, magic, klayout, PDN, routing, the 9T
   `config.tcl` SYNTH_DRIVING_CELL_PIN shim + sc9 site). Apply it into
   `gf180mcuD/libs.tech/librelane/`.
4. **Corners via `common.json`** (KianV-style, 3.x-native): `STD_CELL_LIBRARY:
   gf180mcu_fd_sc_mcu9t5v0`, `LIB` = {corner-glob: [9T lib, IO lib]} at 3v30,
   `DEFAULT_CORNER: nom_tt_025C_3v30`, `STA_CORNERS: [max_*_3v30/3v00/3v60]`.
   SRAM libs go per-macro in each cache/boot `MACROS` def (NOT global LIB).

**Next:** validate the FULL `cpus` P&R (not just synth) on this template to
shake out the 3.x P&R/signoff step names (the `OL_TO`/`OL_SKIP` in run.sh), then
Phase 2 rollout. The overlay strip must also be committed as a `gf180mcuD/`
overlay tree (currently applied ad-hoc into the ciel dir for the test).

## Phase 1 COMPLETE ✅ (2026-07-31)
Full `cpus` P&R runs end-to-end on **LibreLane 3.0.5 + gf180mcuD + 9T/3.3V**
(synth → floorplan → PDN → place → CTS → route → RCX; "voltage from lib (3.3V)"
confirmed; routing overflow 0.13%). **cpus die = 1.57 mm²** at 9T/3.3V (vs
1.24 mm² 7T/5V — the ~27% 9T area cost, as expected).
Additional 3.x overrides needed (all in common.json): `PAD_LIBS: {}` for
pad-less macros (overrides migrate's 5v00 AND dodges io.tcl's raw-dict PAD_LIBS
loop bug at line 240). `OL_TO=Magic.WriteLEF` is NOT honored in 3.x (ran past to
RCX) — the run.sh stop-point step id needs updating for 3.x.

## Phase 2 progress (2026-07-31)
Both cell classes validated on 3.0.5 + gf180mcuD + 9T/3.3V:
- **cpus** (logic): die **1.57 mm²** (7T/5V was 1.24 — the ~27% 9T cost).
- **icache_2k** (SRAM): die **1.8 mm²** (7T/5V was 1.8 — caches are SRAM-bound,
  so library-independent, as expected). `fix_macro_paths` "already match"
  (3.0.5 yosys aligns). SRAM MACRO `lib` fields converted 5v00→3.3V per-corner
  (reusable helper: MACROS[*fd_ip_sram*].lib -> {corner: [sram__<corner>.lib]}).

FLOW-HYGIENE items (don't block area measurement — metrics come from route/RCX):
- `OL_TO=Magic.WriteLEF` no longer stops the 3.x flow cleanly (runs into
  signoff). run.sh's `--to` needs the exact 3.x step id (TBD).
- KLayout **render** step fails on the SRAM cells' CIF boundary (cosmetic) for
  cache macros — add to OL_SKIP or stop before it once the step id is confirmed.

REMAINING (Phase 2): dcache_2k, sdram(smoke), devices, qspi_flash,
mem_region_mux, boot — same recipe (SRAM-lib convert for cache/boot; 9T
floorplan re-tune where a tight 7T die no longer fits).

## Phase 2 COMPLETE ✅ — all 6 pad-ring children at 9T/3.3V on 3.0.5
| macro | 7T/5V | 9T/3.3V | Δ |
|---|---|---|---|
| cpus | 1.24 | 1.57 | +27% (logic) |
| icache_2k | 1.8 | 1.8 | flat (SRAM) |
| dcache_2k | 2.7 | 2.7 | flat (SRAM) |
| sdram | 0.05 | 0.069 | +38% |
| devices | 0.41 | 0.514 | +25% |
| qspi_flash | 0.56 | 0.691 | +23% |
Logic ~+23-38% (9T cells); SRAM-bound caches flat (library-independent). All
harden end-to-end; the KLayout-render step fails on SRAM CIF (cosmetic; die/LEF
produced before it). SRAM MACRO lib fields converted 5v00->3.3V per-corner.

## Phase 3 result (2026-07-31): pad ring at 9T/3.3V = 18.13 mm²
Flat pad ring hardens to CTS + global-route on 3.0.5+D at 9T/3.3V: **die
4120x4400 = 18.13 mm²** (placed/global-routed confirmed) — still BELOW KianV's
20.1 mm² even at the KianV-matched operating point. Top floorplan re-tuned for
the bigger 9T children (die 3120->3400, devices/icache shifted; clean).

KNOWN 3.x LIMITATION (detailed route): 3.0.5's TritonRoute is stricter than
2.4.2 about pins outside CORE_AREA and hard-errors DRT-0073 (no access point)
on the perimeter pads' PAD terminals. The flat-macro pad approach worked on
2.4.2 but conflicts on 3.0.5: CORE_AREA excluding the pad margin -> DRT-0073;
including it -> PDN-0179 (straps can't repair channels around pads). Proper fix
= LibreLane's real IO/pad-ring flow (not the flat-macro shortcut) -- a scoped
follow-up. The die AREA (the KianV comparison metric) is confirmed regardless.

## The REAL pad-ring flow (from KianV) — Chip/Padring flow, no chip-level routing

KianV's `scripts/padring.py` defines a `PadringFlow` (SequentialFlow) whose
steps are: Synthesis -> Floorplan -> SetPowerConnections -> **OpenROAD.PadRing**
-> ManualMacroPlacement -> KLayout.StreamOut -> KLayout.SealRing. **There is NO
GlobalRoute/DetailedRoute/CTS/Placement of std cells.** So chip_top is an
ASSEMBLY of pads + a pre-hardened core MACRO, not a full P&R -- which is exactly
why KianV never hits our DRT-0073 (our flat approach routes the soc glue AT chip
level, so TritonRoute must access the pad pins; KianV's core routing is internal
to the pre-hardened core macro).

3.0.5's `OpenROAD.PadRing` step takes `PAD_SOUTH/EAST/NORTH/WEST` (pad instance
names per edge) + `PAD_CFG`; it places + connects the pads. `PDN_CFG` +
`PDN_CORE_RING`(+CONNECT_TO_PADS) build the core ring that straps to the pads
(fixes our PDN-0179). `CORE_AREA` is inset by the pad depth; the core macro is
placed at `expr::$DIE_AREA - pad_depth`.

**Adoption plan:**
1. Harden the flat soc (6 child macros + glue) as a ROUTED `chip_core` macro
   (run.sh macro=top on 3.0.5, full route+LEF), with I/O pins placed at the
   boundary in the pad order.
2. `chip_top.sv` = pads (bidir + dvdd/dvss + clk/rst, w/ OE/PU/PD control) around
   the `chip_core` macro (adapt KianV's src/chip_top.sv).
3. Copy KianV's PadringFlow (scripts/padring.py) + config (flow, PAD_* lists,
   PDN_CFG core ring, CORE_AREA inset) + pdn_cfg.tcl, adapted to our nets/pads.
4. Run the padring flow -> properly-assembled, sealed, 0-DRC padded die.
