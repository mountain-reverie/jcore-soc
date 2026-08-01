# chip_core — flat whole-soc P&R (KianV-style)

`chip_core` is the entire J4+MMU soc hardened as **one flat block**: all logic
placed as std cells, only the vendor SRAM (`gf180mcu_fd_ip_sram`) left as
(auto-placed) macros. This is the KianV RISC-V topology — a single core P&R
that `chip_top` then wraps in the IO pad ring via the real LibreLane
Chip/Padring flow (no chip-level routing, which is what the earlier flat
`pad_ring` integration tripped over: DRT-0073 on the pad PAD terminals).

Contrast with `top/`, which hardens the same soc **hierarchically** (6 child
macros + glue). `chip_core` trades hierarchy for a single congestion domain so
the pad ring has one macro to abut.

## Regenerating `chip_core.v`

`chip_core.v` is generated: the committed hierarchical netlists
(`top/soc.v` glue + the 6 child `*.v` bodies) flattened into one module, with
the SRAM cells kept as blackbox leaves. Host `yosys` (>=0.44) + the PDK SRAM
blackbox models are all that is needed:

```sh
LB=.                                   # this librelane/ dir
P=$(ls -d "$HOME"/.ciel/ciel/gf180mcu/versions/*/gf180mcuD/libs.ref | head -1)
S=$P/gf180mcu_fd_ip_sram/verilog
yosys -q -p "
  read_verilog -lib \
    $S/gf180mcu_fd_ip_sram__sram64x8m8wm1__blackbox.v \
    $S/gf180mcu_fd_ip_sram__sram512x8m8wm1__blackbox.v;
  read_verilog $LB/top/soc.v $LB/cpus/cpus.v \
    $LB/icache_2k/icache_adapter.v $LB/dcache_2k/dcache_adapter.v \
    $LB/smoke/sdram_ctrl.v $LB/soc_cluster.devices/devices.v \
    $LB/qspi_flash/qspi_flash_ctrl.v;
  hierarchy -top soc;
  flatten;
  opt_clean;
  write_verilog -noattr $LB/chip_core/chip_core.v
"
```

Expected: one `soc` module, **17 SRAM instances** (7 icache + 10 dcache), the
rest std cells. The tri-state warnings during read are the inout pad signals
(`ice_spi_io`, gpio, sd_cmd) and are benign.

## Hardening

```sh
OL_IMAGE=ghcr.io/librelane/librelane:3.0.5 ./run.sh macro=chip_core
```

9T / 3.3 V on LibreLane 3.0.5 (see `common.json`). The SRAM macros are declared
in `config.json` with lef/gds/lib but **no fixed instances** — LibreLane
auto-places them. `DIE_AREA` / `PL_TARGET_DENSITY_PCT` are tuned for the flat
soc; widen the die if placement legalization (DPL) or global route (GRT)
reports congestion.
