# top — hierarchical glue netlist (RETIRED as a P&R flow)

> **This flow was removed on 2026-08-09.** `top/config.json` and the
> `run.sh macro=top` / `macro=pad_ring` branches are gone. What survives in
> this directory is `soc.v` (the top-level interconnect glue) and
> `soc_macros_bb.v` (blackbox stubs) — used ONLY as inputs to
> `../chip_core/gen_chip_core.sh`, which flattens them with the six child
> netlists into `chip_core.v`, the sole RTL input to `chip_top`.
>
> **Why it went.** `top/config.json` hand-placed each child macro at fixed
> coordinates. The J4 sh4-overlay decoder (`a9df9bf`, 2026-08-04) grew `cpus`
> to 1661×1688 µm, which overlapped `devices` by 344×715 µm and
> `icache_adapter` by 1557×368 µm — breaking the PDN (`PSM-0069`) and
> diverging global placement (`GPL-0305`). Absorbing the CPU would have meant
> re-solving the placement by hand and growing the padded die to roughly
> 25 mm², past KianV's 20.1 mm² line.
>
> `chip_top` (LibreLane `Chip` flow) instead hardens the whole SoC flat — only
> the 17 vendor SRAMs placed, pads abutted as a ring — and routes the same
> post-decoder design DRC-clean at **12.92 mm²** with no hand placement at
> all. See `../chip_top/README.md`.
>
> The rest of this file is retained as historical background on the
> hierarchical approach and the `cpus` cluster it introduced.

---

## Historical: GF180 integrated-top P&R (`macro=top`)

`run.sh macro=top` hardens the whole flash-variant `soc` as a hierarchical
black-box floorplan: the 7 hardened sub-blocks (**cpus**, icache_adapter,
dcache_adapter, sdram_ctrl, devices, qspi_flash_ctrl, mem_region_mux) are placed
as LEF macros and only the top-level interconnect glue is real logic. This keeps
peak memory ~0.9 GB (a flat whole-SoC flatten OOM'd).

## The `cpus` cluster macro (core + boot fused)

The CPU and boot ROM are hardened together as **one `cpus` macro** (`run.sh
macro=cpus`, entity `cpus` / arch `one_cpu_m0_gf180`): the J4 core, the pure-ROM
boot memory (`bootram_infer(boot_mem_gf180)` — a read-only boot vector table, no
vendor SRAM), and the `bootmem_onewait_inst`/`_data` bridges, all as std cells
(no macro nesting). This replaces the earlier split of a separate `cpu` macro
plus a lone far-west `boot_mem_top_gf180` macro.

Why: the boot ROM only talks to the CPU (reset instruction/data fetch through the
onewait bridges). Fusing them makes that bus **internal** to the macro instead of
a ~3.2 mm cross-die top-level route to an isolated boot macro, and drops the
top-macro count 8 → 7 (which also removes the tiny-isolated-macro PDN-strap
fiddliness). The `cpus` macro is hardened area-only (`OL_TO=Magic.WriteLEF`, loose
clock — see `../cpus/config.json`); its footprint (~1.26 mm²) is essentially the
same as the old cpu-alone + boot pair, so the top die is unchanged.

The top netlist (`soc.v`) is regenerated with `cpus` black-boxed as a single
module — see "Regenerating" below.

## Floorplan: 2KB dense + pin-order arrangement

`config.json` binds the **2KB dense-hardened** caches (`../icache_2k`,
`../dcache_2k`) and arranges the macros to follow the on-chip bus topology
rather than a rectangular grid:

- **Memory cluster, WEST** — `qspi_flash_ctrl` → `mem_region_mux` → `sdram_ctrl`
  stacked at the west edge (the flash/DDR side).
- **Caches, centre** — `dcache` (bottom) and `icache` (top), right-aligned so
  their pin-ordered **East** edge (`ibus_*`/`a_mmu_*`/`ctrl_*`) faces the CPU and
  their **West** edge (`dbus_*`/`snpc_*`) faces the memory cluster.
- **`cpus`, EAST** — across a 300 µm routing channel from the caches, with
  `devices` stacked to its north. The boot ROM is inside `cpus`, so there is no
  separate far-west boot macro.

Levers compound: (1) dense cache hardening, (2) the caches' own
`FP_PIN_ORDER_CFG` pin placement, (3) topology-driven macro placement, (4) the
cpus-cluster fuse (internalises the boot bus).

### Measured result (real LibreLane P&R, `OL_TO=Magic.WriteLEF`)

| top variant | die | instance | util | route wirelength | macros | DRC |
|---|---|---|---|---|---|---|
| 8 KB-cache grid | 4540×4670 = 21.20 mm² | 12.76 mm² | 61 % | — | 8 | 0 |
| 2 KB dense + pin-order (cpu+boot split) | 4478×4450 = 19.93 mm² | 10.89 mm² | 55.7 % | 1.76 M | 8 | 0 |
| **2 KB dense + pin-order + cpus-fuse** | **4497×4450 = 20.01 mm²** | **10.88 mm²** | 55.4 % | **1.27 M DBU** | **7** | **0** |

Fusing the cpus cluster leaves the die and instance area flat but cuts top-level
wirelength a further **~28 %** (1.76 M → 1.27 M) by internalising the boot
reset-fetch bus and dropping one macro, at 100 % detailed-route completion with 0
violations.

### Superseded arrangement (cpu+boot split)

Before the cpus fuse, the CPU was `cpus.core0.u_cpu` (a separate `cpu` macro) and
the boot ROM was a separate `boot_mem_top_gf180` placed far-west, strap-aligned
(x=180 / y=2160 to catch the PDN straps at 180 µm pitch — a small isolated macro
otherwise trips PDN-0233). That row in the table above (1.76 M wirelength) is the
2 KB top just before this change.

## Committed inputs

- **`soc.v`** — the top-level interconnect netlist (glue only; the 7 macros are
  instances, not flattened). Committed because it is a P&R *input*, not run
  output.
- **`soc_macros_bb.v`** — `(* blackbox *)` empty declarations for the 7 macros
  (`cpus`, the 2 cache adapters, and the 4 peripherals), so LibreLane's Yosys
  hierarchy check resolves the instances without re-deriving their bodies.
- **`config.json`** — points `VERILOG_FILES` at the two files above; `MACROS`
  wires each of the 7 child LEFs (`cpus` LEF/GDS from `../cpus/`).

## Regenerating `soc.v` / `soc_macros_bb.v` (only if the SoC RTL changes)

The netlist is regenerated with a **clean-analyze** recipe (a stale/contaminated
GHDL work library is what made an earlier attempt appear to hit a GHDL synth
crash — a fresh workdir is the fix; no newer GHDL is needed):

1. Regenerate the flash variant fresh (never trust a stale
   `output/gf180_j4mmu/config/config.vhd`):
   `bash targets/asic/gf180_j4mmu/prep_sources.sh && make gf180_j4mmu TARGET=soc_gen VARIANT=flash`
2. Build the synth-clean source set via `metrics/gen_synth_sources.sh` (it
   strips the `soc_port_*` attributes GHDL-yosys chokes on) for the flash
   variant **with the vendor-SRAM rebind** — i.e. the `_gf180` cache adapters
   (`ddr_ram_mux_one_cpu_idcache_gf180`), `bootram_infer(boot_mem_gf180)`, and
   `cpus_one_m0_gf180_arch` — analyzed into a **FRESH** ghdl workdir (delete any
   existing one first).
3. `yosys -m ghdl -p "<GHDL_BASE> -e soc; hierarchy -top soc; blackbox <the 7
   macro modules>; synth -top soc -flatten; write_verilog soc.v"`. The 7 modules
   are `cpus` (the cluster — core + boot + onewait, so cpu and boot are NOT
   black-boxed separately), the 2 cache adapters, and the 4 peripherals.
4. Emit `soc_macros_bb.v` = blackbox stub declarations for the 7 macro modules
   (`write_verilog -blackboxes`).
5. Post-process module names in `soc.v`/`soc_macros_bb.v` to match the child LEF
   `MACRO` names exactly (ghdl mangles them, e.g. `cpus_Bone_cpu_m0_gf180_<hash>`
   → `cpus`). Also normalise escaped record-field names `\<sig>[<field>]` →
   `<sig>_<field>` (leaving numeric vector bits `[0]`.. intact) so the `soc.v`
   instance ports and the stub ports match the macro LEF pin names (which flatten
   VHDL record ports to underscore when the macro is elaborated as its own top).

## Known finishing limitation

The flow stops before `Magic.StreamOut` (final GDS merge): that step needs each
child macro's **GDS** with a PR-boundary layer, but the children were hardened
to LEF only (`OL_TO=Magic.WriteLEF`). The routed **area** (die/core/instance,
from the CTS/route step `or_metrics_out.json`) is complete without it; a full
merged GDS would require re-hardening the children through GDS.
