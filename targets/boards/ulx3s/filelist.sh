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
  # M1b: cache + ddr_ram_mux + dma (depend on cpu2j0_pack + data_bus_pack)
  components/ddr2/ddrc_cnt_pkg.vhd
  components/cpu/cache/cache_pkg.vhd
  lib/reg_file_struct/bist_pkg.vhd
  components/dma/dma_pkg.vhd
  lib/memory_tech_lib/memory_pkg.vhd
  lib/memory_tech_lib/ram_1rw.vhd
  lib/memory_tech_lib/ram_2rw.vhd
  lib/memory_tech_lib/tech/inferred/ram_1rw_infer.vhd
  lib/memory_tech_lib/tech/inferred/ram_2rw_infer.vhd
  components/misc/bus_mux_pkg.vhd
  components/misc/bus_mux_ff_pkg.vhd
  components/misc/bus_mux_lock_pkg.vhd
  components/misc/bus_mux_typec_pkg.vhd
  components/cpu/cache/dcache_adapter.vhd
  components/cpu/cache/icache_adapter.vhd
  components/cpu/cache/dcache_ram.vhd
  components/cpu/cache/icache_ram.vhd
  components/cpu/cache/dcache_ccl.vhd
  components/cpu/cache/dcache_mcl.vhd
  components/cpu/cache/icache_ccl.vhd
  components/cpu/cache/icache_mcl.vhd
  targets/boards/ulx3s/generated/dcache.vhd  # transparent latch -> negedge FF
  targets/boards/ulx3s/generated/icache.vhd  # (see gen_synth_sources.sh)
  components/cpu/cache/cache_config_fpga.vhd
  components/misc/bus_mux_typecsub.vhd
  components/misc/bus_mux_typec.vhd
  targets/boards/ulx3s/generated/ddr_ram_mux.vhd  # soc_gen metadata stripped
  targets/ddr_ram_mux/one_cpu_idcache.vhd
  targets/ddr_ram_mux/one_cpu_idcache_fpga.vhd
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
  components/sdram/sdram_pkg.vhd
  components/sdram/sdram_ctrl.vhd
  components/sdram/sdram_iocells.vhd
  targets/boards/ulx3s/ulx3s_clkgen.vhd
  targets/boards/ulx3s/cpus_one_m0.vhd
  targets/boards/ulx3s/ulx3s_top.vhd
)
