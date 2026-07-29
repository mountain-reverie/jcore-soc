# GF180 integrated-top P&R (`macro=top`)

`run.sh macro=top` hardens the whole flash-variant `soc` as a hierarchical
black-box floorplan: the 8 hardened sub-blocks (cpu, icache_adapter,
dcache_adapter, boot_mem_top_gf180, sdram_ctrl, devices, qspi_flash_ctrl,
mem_region_mux) are placed as LEF macros and only the top-level interconnect
glue (~760 cells) is real logic. This keeps peak memory ~0.9 GB (a flat
whole-SoC flatten OOM'd).

## Floorplan: 2KB dense + pin-order arrangement

`config.json` binds the **2KB dense-hardened** caches (`../icache_2k`,
`../dcache_2k`) and arranges the macros to follow the on-chip bus topology
rather than a rectangular grid:

- **Memory cluster, WEST** — `qspi_flash_ctrl` → `mem_region_mux` → `sdram_ctrl`
  stacked at the west edge (the flash/DDR side).
- **Caches, centre** — `dcache` (bottom) and `icache` (top), right-aligned so
  their pin-ordered **East** edge (`ibus_*`/`a_mmu_*`/`ctrl_*`) faces the CPU and
  their **West** edge (`dbus_*`/`snpc_*`) faces the memory cluster.
- **CPU, EAST** — `cpus.core0.u_cpu` across a 300 µm routing channel from the
  caches, with `devices` stacked to its north.
- **boot ROM** — far-west, strap-aligned (x=180 / y=2160 catch the PDN straps at
  180 µm pitch; a small isolated macro otherwise trips PDN-0233).

Three levers compound: (1) dense cache hardening, (2) the caches' own
`FP_PIN_ORDER_CFG` pin placement, (3) topology-driven macro placement.

### Measured result (real LibreLane P&R, `OL_TO=Magic.WriteLEF`)

| top variant | die | instance | util | route wirelength | DRC |
|---|---|---|---|---|---|
| prior 8 KB-cache grid | 4540×4670 = 21.20 mm² | 12.76 mm² | 61 % | — | 0 |
| **2 KB dense + pin-order** | **4478×4450 = 19.93 mm²** | **10.89 mm²** | 55.7 % | **1.76 M DBU** | **0** |

The instance area (real silicon + macros) drops **15 %** below the 8 KB grid and
the die **6 %**, at 100 % detailed-route completion with 0 violations. The
tighter die still routes because the pin-ordered caches + topology placement cut
top-level wirelength ~24 % vs a loose 2 KB arrangement.

## Committed inputs

- **`soc.v`** — the top-level interconnect netlist (glue only; the 8 macros are
  instances, not flattened). Committed because it is a P&R *input*, not run
  output.
- **`soc_macros_bb.v`** — `(* blackbox *)` empty declarations for the 8 macros +
  the 2 leaf vendor-SRAM cell types, so LibreLane's Yosys hierarchy check
  resolves the instances without re-deriving their bodies.
- **`config.json`** — points `VERILOG_FILES` at the two files above; `MACROS`
  wires each of the 8 child LEFs.

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
3. `yosys -m ghdl -p "<GHDL_BASE> -e soc; hierarchy -top soc; blackbox <the 8
   macro modules>; synth -top soc -flatten; write_verilog soc.v"`.
4. Emit `soc_macros_bb.v` = blackbox stub declarations for the 8 macro modules +
   the `sram256x8`/`sram512x8` cell types.
5. Post-process module names in `soc.v` to match the child LEF `MACRO` names
   exactly (ghdl mangles them).

## Known finishing limitation

The flow stops before `Magic.StreamOut` (final GDS merge): that step needs each
child macro's **GDS** with a PR-boundary layer, but the children were hardened
to LEF only (`OL_TO=Magic.WriteLEF`). The routed **area** (die/core/instance,
from the CTS/route step `or_metrics_out.json`) is complete without it; a full
merged GDS would require re-hardening the children through GDS.
