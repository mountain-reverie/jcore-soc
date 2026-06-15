# Canonical ghdl analyze order for the ULX3S M0 design.
# Sourced by sim.sh and synth.sh (after cd to repo root). Defines FILES.
# The cpu .vhd files must be generated first: `make -C components/cpu/decode
# generate` + v2p of mult/datapath/decode_core (done by sim.sh/synth.sh).
CPU=components/cpu
FILES=(
  $CPU/cpu2j0_pkg.vhd
  $CPU/core/components_pkg.vhd
  $CPU/core/mult_pkg.vhd
  $CPU/decode/decode_pkg.vhd
  $CPU/core/datapath_pkg.vhd
  $CPU/core/cpu.vhd
  $CPU/core/mult.vhd
  $CPU/core/datapath.vhd
  $CPU/core/register_file.vhd
  $CPU/core/register_file_flops.vhd
  $CPU/core/register_file_two_bank.vhd
  $CPU/decode/decode.vhd
  $CPU/decode/decode_body.vhd
  $CPU/decode/decode_table.vhd
  $CPU/decode/decode_table_direct.vhd
  $CPU/decode/decode_core.vhd
  $CPU/decode/decode_table_direct_config.vhd
  $CPU/synth/cpu_synth_config.vhd
  lib/hwutils/attr_pkg.vhd
  components/misc/misc_pkg.vhd
  targets/boards/ulx3s/config.vhd
  targets/data_bus_pkg.vhd
  targets/cpu_core_pkg.vhd
  targets/cpu_core.vhd
  targets/cpus.vhd
  lib/hwutils/data_bus_delay.vhd
  lib/hwutils/instr_bus_delay.vhd
  components/uartlite/uart_pkg.vhd
  components/uartlite/uart.vhd
  components/uartlite/uartlitedb.vhd
  targets/boards/ulx3s/boot_image_pkg.vhd
  components/memory/bootram_infer.vhd
  targets/boards/ulx3s/ulx3s_clkgen.vhd
  targets/boards/ulx3s/cpus_one_m0.vhd
  targets/boards/ulx3s/ulx3s_top.vhd
)
