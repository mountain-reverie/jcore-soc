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
| core model  | J1, J2, J4, J4C       | `cpu_synth_{j1, direct, j4, …}` (mult + regfile + shifter bundle, + cache/TLB for J4C) |
| decode      | direct, rom           | `cpu_decode_{direct,rom}_fpga` + which `decode_table_*` file compiled |
| coprocessor | on/off                | `COPRO_DECODE` generic on `cpu_core`                          |

The `cpus` **entity interface is identical** regardless of core count (it always has
cpu0 and cpu1 ports; `one_cpu` ties cpu1 off). A two-core config additionally
instantiates `cpumreg` (RAM arbitration / CPU1 enable).

J4 and J4C are real core models in the `components/cpu` submodule (mountain-reverie
master): J4 is an SH-4 superset of the J1/J2 instruction set; **J4C adds cache + TLB**.
(J1 and J2 share the same instruction set — they differ only in microarchitecture, which
is why the field is named `model`, not `isa`.) The submodule must be pinned to a
revision that contains J4/J4C.

## Design

### 1. Declarative `cpu:` block

A new `cpu:` block in the board YAML:

```yaml
cpu:
  cores: 2          # 1 | 2          → architecture one_cpu | two_cpus_fpga
  model: j4c        # j1 | j2 | j4 | j4c   (j4c implies cache + TLB)
  decode: rom       # direct | rom   → cpu_decode_* config + decode_table_* file
  copro: false      # → COPRO_DECODE generic
```

`model` selects the core microarchitecture bundle (multiplier, register file, shifter,
and — for `j4c` — cache + TLB). The cache/TLB is implied by `model: j4c`; there is no
separate `cache:` flag.

socgen consumes this block and produces:

1. **A generated `cpus_config.vhd`** — the full `configuration <name> of cpus is …`
   declaration that today is hand-written as `cpus_one_m0.vhd`. The cascade is emitted
   from the axes: core-count → architecture, `model` → `cpu_synth_*` binding, `decode`
   → `cpu_decode_*` binding, `copro` → generic.
2. **The CPU portion of the synthesis filelist** — which `decode_table_*` and
   `cpu_synth_*` source files to compile (plus `cpumreg` for two-core). This removes
   decode/synth file selection from the hand-written `filelist.sh`.

**Stable generated configuration name.** The generated configuration always uses one
fixed name (e.g. `work.soc_cpus_config`). The hand-written top-level references that
single name and never changes between variants — only the generated *body* changes.
This decouples the hand-written board top from the chosen CPU topology.

### 2. Multi-variant boards (separate YAML + `!include` base)

```
targets/boards/ulx3s/
  base.yaml               # board-physical: pins, devices, clocks, system, padring
  design.j2-single.yaml   # !include base.yaml  +  cpu: {cores:1, model:j2, decode:direct}
  design.j4c-single.yaml  # !include base.yaml  +  cpu: {cores:1, model:j4c, decode:rom}
  design.j4-dual.yaml     # !include base.yaml  +  cpu: {cores:2, model:j4, decode:rom}
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
- **CPU** — `cpu: {cores:1, model:j1, decode:rom, copro:false}` (J1 fits the iCE40 LUT
  budget).

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

- Pin `components/cpu` submodule to a mountain-reverie revision containing J4/J4C.
- Confirm the exact `cpu_synth_*` / `cpu_decode_*` config names for J4 and J4C in the
  pinned submodule before wiring the model→config map.
- Decide where the generated `cpus_config.vhd` is written (board output dir) and how the
  hand-written top references it via the stable name.
