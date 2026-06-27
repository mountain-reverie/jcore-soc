# Declarative CPU Topology, Multi-Variant Boards, and iCE40 Target

Date: 2026-06-27
Status: Design — approved for planning

## Goal

Make CPU topology declarative in the board YAML so new board configurations can be
expressed without hand-writing VHDL `configuration` declarations or per-board
filelist logic. Support, on top of that refactor:

- ULX3S with a single **J4C** core (cache + TLB).
- ULX3S multi-configuration variants: 1×J2, 2×J2, 1×J4, 2×J4, 1×J4C, etc.
- iCESugar (Lattice **iCE40**) board running a **J1** core, including a new PCF
  constraint emitter and iCE40 clock/synth flow.
- **Zero regression**: existing boards (turtle_1v0, mimas_v2, microboard) keep
  building byte-identical generated VHDL until intentionally migrated.

This is a refactor-first effort: the declarative CPU block is built first, then the
boards are brought up on top of it.

## Background — current mechanism

The Go socgen tool (`tools/socgen/`) already generates `soc.vhd`, `devices.vhd`,
memory map, IRQ/AIC wiring, device tree, and C headers from a YAML board definition,
and emits ECP5 `.lpf` constraints (`emit.LPF`). What it does **not** do today:

- **CPU topology is not declarative.** The YAML only *names* a pre-built VHDL
  `configuration` (e.g. `one_cpu_m0_direct_fpga`). The actual selection of core
  count, core model, decode style, and coprocessor support lives in hand-written
  VHDL (`targets/cpus_one.vhd`, `targets/cpus_two_fpga.vhd`,
  `targets/boards/ulx3s/cpus_one_m0.vhd`) plus per-board `filelist.sh` file
  selection. The hand-written top-level (`ulx3s_top.vhd:95`) hard-codes the
  configuration name.
- **No iCE40 / PCF support.** Only ECP5 (LPF, generated) and Spartan6 (hand-written
  UCF) exist.
- **No multi-variant board concept.** Each board dir = one `design.yaml`.

### CPU topology is fully captured by orthogonal axes

The exploration of the VHDL config layer established that a `cpus` configuration is a
deterministic cascade over these axes:

| Axis        | Values                | Today's selection mechanism                                  |
|-------------|-----------------------|--------------------------------------------------------------|
| core count  | 1, 2                  | architecture `one_cpu` vs `two_cpus_fpga` (same `cpus` entity)|
| core model  | J1, J2, J4            | `cpu_synth_{j1, direct, j4}` (mult + regfile + shifter bundle; J4 adds SH-4 privileged datapath + TLB via `PRIV_ARCH`/`MMU_ARCH` generics) |
| decode      | direct, rom           | `cpu_decode_{direct,rom}_fpga` + which `decode_table_*` file compiled |
| coprocessor | on/off                | `COPRO_DECODE` generic on `cpu_core`                          |
| cache       | none, i, id           | **separate axis** — the `ddr_ram_mux` configuration (`ddr_ram_mux_one_cpu_{direct,icache,idcache}_fpga`) + `icache_modereg` byte-bus slave + cache-control registers |

The `cpus` **entity interface is identical** regardless of core count (it always has
cpu0 and cpu1 ports; `one_cpu` ties cpu1 off). A two-core config additionally
instantiates `cpumreg` (RAM arbitration / CPU1 enable).

**Cache is orthogonal to the CPU model.** In the SoC, cache is *not* part of the CPU
synth config (the submodule's `cpu_cache_timing_top` is only a measurement harness).
It is selected by the **`ddr_ram_mux` top-entity block**, which — like `cpus` — already
names a VHDL `configuration` in `design.yaml`. ULX3S today already runs an I+D cache
(`ddr_ram_mux_one_cpu_idcache_fpga`) paired with a J2-direct CPU. So "J4C" is **not new
cache integration**: it is `model: j4` (SH-4 privileged + TLB/MMU binding) combined with
the already-integrated `cache: id` ram-mux.

J4 is a real core model in the `components/cpu` submodule (mountain-reverie master): an
SH-4 superset of the J1/J2 instruction set with privileged datapath + TLB/MMU. (J1 and
J2 share the same instruction set — they differ only in microarchitecture, which is why
the field is named `model`, not `isa`.) **PRIV_ARCH/MMU_ARCH must be set via a VHDL
configuration binding with generic map**, not the ghdl `-g` flag, because the yosys-ghdl
plugin does not support `-g`; the SoC's `cpu_core`→`cpu` component instantiation provides
the binding context. The submodule is pinned to latest mountain-reverie master.

## Design

### 1. Declarative `cpu:` block

A new `cpu:` block in the board YAML:

```yaml
cpu:
  cores: 2          # 1 | 2          → architecture one_cpu | two_cpus_fpga
  model: j4         # j1 | j2 | j4   (j4 ⇒ SH-4 privileged datapath + TLB/MMU binding)
  decode: rom       # direct | rom   → cpu_decode_* config + decode_table_* file
  copro: false      # → COPRO_DECODE generic
  cache: id         # none | i | id  → ddr_ram_mux configuration + icache byte-bus slave
```

`model` selects the core microarchitecture bundle (multiplier, register file, shifter;
for `j4`, additionally binds `PRIV_ARCH`/`MMU_ARCH => true` for the SH-4 privileged
datapath + TLB). `cache` is an **independent axis** selecting the `ddr_ram_mux` variant.
"J4C" = `model: j4` + `cache: id`.

socgen consumes this block and produces:

1. **A generated `cpus_config.vhd`** — the full `configuration <name> of cpus is …`
   declaration that today is hand-written as `cpus_one_m0.vhd`. The cascade is emitted
   from the axes: core-count → architecture, `model` → `cpu_synth_*` binding (with
   `PRIV_ARCH`/`MMU_ARCH` generic map for j4), `decode` → `cpu_decode_*` binding,
   `copro` → generic.
2. **The CPU portion of the synthesis filelist** — which `decode_table_*` and
   `cpu_synth_*` source files to compile (plus `cpumreg` for two-core). This removes
   decode/synth file selection from the hand-written `filelist.sh`.
3. **The `ddr_ram_mux` configuration selection** from `cache:` — mapping `none|i|id`
   to `ddr_ram_mux_one_cpu_{direct,icache,idcache}_fpga` (or the `two_cpu_idcache`
   variant for `cores: 2`), plus the matching ram-mux/cache source files in the
   filelist and the `icache_modereg` byte-bus slave + cache-control registers. This
   axis is already partly declarative (ULX3S names the configuration today); the
   refactor derives it from `cache:` instead of a hand-written configuration string.

**Stable generated configuration name.** The generated configuration always uses one
fixed name (e.g. `work.soc_cpus_config`). The hand-written top-level references that
single name and never changes between variants — only the generated *body* changes.
This decouples the hand-written board top from the chosen CPU topology.

### 2. Multi-variant boards (separate YAML + `!include` base)

```
targets/boards/ulx3s/
  base.yaml               # board-physical: pins, devices, clocks, system, padring
  design.j2-single.yaml   # !include base.yaml  +  cpu: {cores:1, model:j2, decode:direct, cache:id}
  design.j4c-single.yaml  # !include base.yaml  +  cpu: {cores:1, model:j4, decode:rom, cache:id}
  design.j4-dual.yaml     # !include base.yaml  +  cpu: {cores:2, model:j4, decode:rom, cache:id}
```

- A variant file is a thin `!include base.yaml` plus the `cpu:` block (and any
  per-variant override).
- The build selects the variant: `make ulx3s VARIANT=j4c-single` → socgen consumes
  `design.j4c-single.yaml`. The board `Makefile` names a default `VARIANT`.
- This reuses the existing `!include` loader. **No new YAML schema** beyond the `cpu:`
  block.
- A board with a single plain `design.yaml` and no variant files is treated as
  variant-less (no behavior change).

### 3. iCE40 / iCESugar target (full path)

- **`target: ice40`** added to the family switch alongside `ecp5` / `spartan6`.
- **New `emit/pcf.go`** — PCF constraint emitter (`set_io <signal> <pin>`), driven by
  the same resolved-pin model that feeds `emit.LPF`. The YAML `pins.rules` stay
  identical; only the output format differs.
- **iCE40 clkgen VHDL** — a `SB_PLL40_*`-based `clkgen` entity (hand-written board
  VHDL, mirroring ULX3S's ECP5 clkgen), wired through the existing
  `padring-entities: clkgen` mechanism.
- **Synth flow** — `synth_ice40` + `nextpnr-ice40` + `icepack` in the board's
  `synth.sh`, mirroring ULX3S's yosys/ghdl flow.
- **CPU** — `cpu: {cores:1, model:j1, decode:rom, copro:false, cache:none}` (J1, no
  cache, fits the iCE40 LUT budget).

### 4. Testing / no-regression strategy

- **Golden-file tests** in socgen (`emit`/`generate` already have `_test.go`): snapshot
  the generated `cpus_config.vhd` + filelist for representative variants.
- **Regression gate**: regenerate all existing boards and assert no diff in their
  generated VHDL before any migration. Existing boards remain on their current
  single-file `design.yaml` and unchanged generated output until intentionally split.
- **Simulation**: `make check` (GHDL) continues to pass; add a sim build of at least
  one new variant per touched board.
- **iCE40**: PCF emitter unit-tested against the resolved-pin model; a smoke synth of
  iCESugar-J1 to confirm the nextpnr flow.

## Out of scope

- Implementing new CPU hardware. J4/J4C come from the `components/cpu` submodule; this
  effort only makes them selectable and generates the wiring.
- Migrating existing boards to the variant layout (done later, opt-in, gated by the
  no-regression check).
- Auto-generating board top-level entities or hand-written clkgen VHDL.

## Open implementation notes

- `components/cpu` pinned to latest mountain-reverie master (the revision with the real
  J4 privileged datapath + TLB and the `j2c`/`j4c` cache harness).
- Confirmed config names in the pinned submodule: `cpu_synth_direct` (J2),
  `cpu_synth_j1`, `cpu_synth_j4` (+ `cpu_synth_j4_priv` wrapper for `PRIV_ARCH=>true`);
  decode `cpu_decode_{direct,rom}_fpga`. J4 SoC binding sets `PRIV_ARCH`/`MMU_ARCH` via
  configuration generic map at the `cpu_core`→`cpu` instantiation (no `-g`, per the
  yosys-ghdl limitation).
- Cache `none|i|id` → `ddr_ram_mux_one_cpu_{direct,icache,idcache}_fpga` (and the
  `two_cpu_idcache` variant for two cores). ULX3S already integrates `idcache`, so the
  cache path is proven; the refactor only derives the configuration name from `cache:`.
- Decide where the generated `cpus_config.vhd` is written (board `generated/` dir, as
  `ddr_ram_mux.vhd` already is) and how the hand-written top references it via the
  stable configuration name.
