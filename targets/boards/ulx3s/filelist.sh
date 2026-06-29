# Canonical ghdl analyze order for the ULX3S M0 design.
# Sourced by sim.sh and synth.sh (after cd to repo root). Defines FILES.
# The cpu .vhd files must be generated first: `make -C components/cpu/decode
# generate` + v2p of mult/datapath/decode_core (done by sim.sh/synth.sh).
CPU=components/cpu
GEN=targets/boards/ulx3s/generated
# Variant-specific synth sources (decode tables + cpu_synth config + j1/j4
# alternate architectures) are soc_gen-generated into $GEN/cpu_synth_files.list
# (cpu-submodule-relative, one per line; staged by gen_synth_sources.sh). They
# replace the hardcoded j2-direct decode_table_direct/_config + cpu_synth_config
# lines so j1/j4 analyze their own tables. Spliced in below right after
# decode_core.vhd (the position those lines occupied).
[ -f "$GEN/cpu_synth_files.list" ] || { echo "ERROR: $GEN/cpu_synth_files.list missing — run gen_synth_sources.sh (soc_gen) first" >&2; exit 1; }
FILES=(
  $CPU/cpu2j0_pkg.vhd
  $CPU/core/components_pkg.vhd
  # tlb: cpu.vhd directly instantiates work.tlb (entity inst. in the MMU_ARCH
  # generate), so ghdl needs it analyzed before cpu.vhd for ALL variants.
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
# Splice in the soc_gen-generated variant synth sources, $CPU-prefixed.
while IFS= read -r _f; do
  [ -n "$_f" ] && FILES+=("$CPU/$_f")
done < "$GEN/cpu_synth_files.list"
FILES+=(
  lib/hwutils/attr_pkg.vhd
  components/misc/misc_pkg.vhd
  # NB: work.config + work.clk_config are the soc_gen-generated packages
  # (output/<board>/config/config.vhd + targets/clk_config.vhd), analyzed by the
  # consumer scripts (sim.sh/synth.sh) BEFORE this FILES list. The old
  # hand-written stand-in config.vhd is retired: pad_ring/soc/devices need the
  # full generated CFG_CLK_* set (CFG_CLK_PLLE2_HZ, *_PERIOD_NS, ...).
  targets/data_bus_pkg.vhd
  # M1b: cache + ddr_ram_mux + dma (depend on cpu2j0_pack + data_bus_pack)
  components/ddr2/ddrc_cnt_pkg.vhd
  components/cpu/cache/cache_clkmode_sc.vhd  # CACHE_SAME_CLOCK=true -> posedge _sc CDC
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
  components/cpu/cache/dcache.vhd  # posedge _sc CDC (cache_clkmode_sc); Part B
  components/cpu/cache/icache.vhd
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
  # M2: AIC v1 (interrupt controller + RTC + PIT) + peripheral bus mux
  components/misc/aic2_pkg.vhd
  components/misc/aic_edgedet.vhd
  components/misc/aic.vhd
  components/misc/gpio2.vhd
  components/misc/spi2.vhd
  targets/boards/ulx3s/periph_mux.vhd
  targets/boards/ulx3s/boot_image_pkg.vhd
  components/memory/bootram_infer.vhd
  components/sdram/sdram_pkg.vhd
  components/sdram/sdram_ctrl.vhd
  components/sdram/sdram_iocells.vhd
  targets/boards/ulx3s/ulx3s_clkgen.vhd
  targets/boards/ulx3s/cpus_one_m0_arch.vhd
  # soc_gen-generated cpus configuration (soc_cpus_config) replacing the retired
  # hand-written one_cpu_m0_direct_fpga; binds cpu_synth_direct for the j2-direct
  # default variant. Must follow the cpus entity + one_cpu_m0 arch + cpu_synth.
  targets/boards/ulx3s/generated/cpus_config.vhd
  # padring entities soc_gen instantiates but does not emit (leaves).
  targets/boards/ulx3s/reset_sync.vhd
  targets/boards/ulx3s/aic_irq_gen.vhd
  # the soc_gen-generated trio (leaf-first: devices <- soc <- pad_ring), now the
  # synthesized/elaborated board top (replaces the retired hand-written
  # ulx3s_top.vhd). The ECP5 clkgen arch (clkgen(ecp5)/EHXPLLL) that pad_ring
  # binds is appended by the consumer script (real primitive for synth, the
  # tb/ehxpll_sim.vhd stand-in for sim).
  targets/boards/ulx3s/devices.vhd
  targets/boards/ulx3s/soc.vhd
  targets/boards/ulx3s/pad_ring.vhd
)
