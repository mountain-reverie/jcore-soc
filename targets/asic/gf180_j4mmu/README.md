# gf180_j4mmu

GF180 ASIC scaffold target for soc_gen. Single J4 core (SH-4-class MMU is in
the J4 RTL and is software-enabled by Linux `head_32.S` -- no MMU config knob
needed here), direct decode (ASIC-designed combinational decoder; drops the
microcode ROM), id-cache.

Unlike the FPGA boards under `targets/boards/`, this target has **no
pad_ring FPGA primitives, no PLL, no board pin constraints (.lpf/.pcf)**.
soc_gen always derives the `soc` entity's ports from signals that touch a
`pin`/`padring`/`expose` context (see `tools/socgen/elaborate/categorize.go`);
`base.yaml` here omits the FPGA-only `padring-entities:` section (ulx3s's
`clkgen`/`reset_sync`, which bind the ECP5 `EHXPLLL` PLL + reset
synchronizer) but keeps a **logical** `pins:` section (see
`targets/pins/gf180_j4mmu.pins`): one bare `signal: true` catch-all rule maps
each net name straight through with no `Pad` column, so soc_gen still marks
`clk_sys`/`reset`/the SDRAM/UART/SPI/GPIO nets as `soc` boundary ports
(direction auto-inferred from existing drivers -- see
`tools/socgen/elaborate/pins.go:bareSignalDir`), but emits no LOC/IOSTANDARD
attributes and (since `target: gf180` is neither `ecp5` nor `ice40`) no
`.lpf`/`.pcf` file. `pad_ring.vhd` is still generated (soc_gen emits it
unconditionally) but this target does not use it -- GHDL only analyzes/
elaborates `soc.vhd`/`devices.vhd`/`cpus_config.vhd`, never `pad_ring.vhd`.

`targets/boards/gf180_j4mmu` is a symlink to this directory: soc_gen's board
loader and the top-level Makefile's board discovery both hardcode
`targets/boards/<name>`, so the symlink lets this ASIC target reuse that
machinery unmodified.

## Regenerating the SoC

```
make gf180_j4mmu TARGET=soc_gen
```

This regenerates `soc.vhd`, `devices.vhd`, `pad_ring.vhd` (unused by this
target -- see above), `cpus_config.vhd`, `cpu_synth_files.list`, `build.mk`,
`board.dts`, `board.h` in this directory.

## GHDL elaboration

See `filelist.sh` for the file list (adapted from
`targets/boards/ulx3s/filelist.sh`: drops pad_ring/clkgen/PLL and the
dual-core-only/attribute-stripped entries; adds the j4 direct-decode table +
cpu_synth config, `targets/asic/gf180_j4mmu/boot_image_pkg.vhd` -- an
all-zero placeholder ROM, NOT a working bootloader, see that file's header
comment -- and this target's own `devices.vhd`/`cpus_config.vhd`/`soc.vhd`).
`-fexplicit -fsynopsys` are required (same as `targets/boards/ulx3s/sim.sh`)
for `std_logic_unsigned`/overloaded-function usage in the shared RTL.

**Note:** switching `decode: rom` -> `decode: direct` (this target now uses
the ASIC-designed combinational decoder) is expected to step UP the `j4_core`
cell/area numbers in any synth dashboard (~110 KB generated
`decode_table_direct.vhd` vs ~37 KB for `decode_table_rom.vhd`): this is real
combinational logic replacing what was effectively inferred-ROM-as-flops, not
an area regression.

```
cd /home/cedric/work/jcore/jcore-soc
make gf180_j4mmu TARGET=soc_gen   # generates config/config.vhd under output/gf180_j4mmu/
ghdl -a --std=93 -fexplicit -fsynopsys --workdir=WORKDIR \
  output/gf180_j4mmu/config/config.vhd targets/clk_config.vhd \
  $(bash targets/asic/gf180_j4mmu/filelist.sh)
ghdl -e --std=93 -fexplicit -fsynopsys --workdir=WORKDIR soc
```
