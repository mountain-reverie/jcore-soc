# Canonical ghdl analyze order for the iCESugar EBR-only J1 design.
# Sourced by sim.sh and synth.sh (after cd to repo root). Defines FILES.
# The cpu .vhd files must be generated first: `make -C components/cpu/decode
# generate` + v2p of mult/datapath/decode_core and gpio2/uart (done by
# sim.sh/synth.sh). `make icesugar TARGET=soc_gen` must have produced
# cpus_config.vhd + cpu_synth_files.list + devices.vhd + soc.vhd.
CPU=components/cpu
BRD=targets/boards/icesugar
# The J1 variant synth sources (EBR register file, sequential mult/shifter, ROM
# decode table + config, cpu_synth_j1 config) are soc_gen-generated, one per
# line, cpu-submodule-relative.
[ -f "$BRD/cpu_synth_files.list" ] || { echo "ERROR: $BRD/cpu_synth_files.list missing — run make icesugar TARGET=soc_gen first" >&2; exit 1; }
FILES=(
  $CPU/cpu2j0_pkg.vhd
  $CPU/core/components_pkg.vhd
  # tlb: cpu.vhd directly instantiates work.tlb in the MMU_ARCH generate, so
  # ghdl needs it analyzed before cpu.vhd for all variants.
  $CPU/core/tlb.vhd
  $CPU/core/mult_pkg.vhd
  $CPU/decode/decode_pkg.vhd
  $CPU/core/datapath_pkg.vhd
  $CPU/core/cpu.vhd
  $CPU/core/mult.vhd
  $CPU/core/datapath.vhd
  $CPU/core/shifter.vhd
  $CPU/core/register_file.vhd
  $CPU/core/register_file_flops.vhd
  $CPU/core/register_file_two_bank.vhd
  $CPU/decode/decode.vhd
  $CPU/decode/decode_body.vhd
  $CPU/decode/decode_table.vhd
  $CPU/decode/decode_core.vhd
)
# Splice in the soc_gen-generated J1 synth sources, $CPU-prefixed.
while IFS= read -r _f; do
  [ -n "$_f" ] && FILES+=("$CPU/$_f")
done < "$BRD/cpu_synth_files.list"
FILES+=(
  lib/hwutils/attr_pkg.vhd
  components/misc/misc_pkg.vhd
  # The full clock/config constants (CFG_CLK_CPU_PERIOD_NS, CFG_CLK_PLLE2_HZ,
  # CFG_CLK_MEM_PERIOD_NS, CFG_CLK_BITLINK_PERIOD_NS) the generated soc/devices/
  # clk_config consume come from the soc_config.mk-generated config package.
  output/icesugar/config/config.vhd
  targets/clk_config.vhd
  targets/data_bus_pkg.vhd
  targets/cpu_core_pkg.vhd
  targets/cpu_core.vhd
  targets/cpus.vhd
  # EBR boot RAM (all memory) + its boot image.
  $BRD/boot_image_pkg.vhd
  components/memory/bootram_infer.vhd
  # Peripherals served by the generated devices.vhd: uartlite + gpio2 + the
  # multi-master peripheral bus mux.
  components/uartlite/uart_pkg.vhd
  components/uartlite/uart.vhd
  components/uartlite/uartlitedb.vhd
  components/misc/bus_mux_pkg.vhd
  components/misc/multi_master_bus_mux.vhd
  components/misc/gpio2.vhd
  # soc_gen-generated SoC: the EBR-only cpus arch + its soc_cpus_config (binds
  # cpu_synth_j1) must precede soc.vhd; devices.vhd precedes soc.vhd.
  $BRD/cpus_one_ebr.vhd
  $BRD/cpus_config.vhd
  $BRD/devices.vhd
  $BRD/soc.vhd
  # Board top (soc_gen-generated pad_ring) + 12 MHz clkgen.
  $BRD/ice_clkgen.vhd
  $BRD/pad_ring.vhd
)
