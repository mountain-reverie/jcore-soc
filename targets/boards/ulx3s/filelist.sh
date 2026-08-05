# Canonical ghdl analyze order for the ULX3S M0 design.
# Sourced by sim.sh and synth.sh (after cd to repo root). Defines FILES.
# The cpu .vhd files must be generated first: `make -C components/cpu/decode
# generate` + v2p of mult/datapath/decode_core (done by sim.sh/synth.sh).
CPU=components/cpu
GEN=targets/boards/ulx3s/generated
# Variant-specific synth sources (decode_pkg/decode/decode_body + the
# selected decode table + its config + cpu_synth config + j1/j4 alternate
# architectures) are soc_gen-generated into $GEN/cpu_synth_files.list
# (cpu-submodule-relative, one per line; staged by gen_synth_sources.sh).
#
# ALL SIX cpugen outputs (decode_pkg.vhd, decode.vhd, decode_body.vhd, and
# the three decode_table_*.vhd) are emitted by ONE cpugen invocation and
# share layout constants (e.g. decode_pkg.vhd's DEC_ADDR_BITS must match the
# selected decode_table_<kind>.vhd's ROM geometry) -- see
# tools/socgen/elaborate/cpumap.go's decodeGenFiles comment. So
# decode_pkg.vhd/decode.vhd/decode_body.vhd must ALSO come from
# cpu_synth_files.list (the model's gen/<model>/decode/ directory), not be
# hardcoded to the committed base decode/ tree here -- hardcoding them
# was exactly the bug a prior round of review found: j4-rom's list pointed
# decode_table_rom.vhd at gen/j4/decode/ while this file still hardcoded the
# BASE decode_pkg/decode/decode_body, wiring two different table geometries
# to the same decoder shell.
#
# decode_pkg.vhd must be analyzed before cpu.vhd (which `use`s its types);
# decode.vhd/decode_body.vhd must be analyzed after cpu.vhd/mult.vhd/
# datapath.vhd but before decode_table.vhd (NOT cpugen-generated, unlike the
# other five -- see decode/Makefile's GENERATED list) and decode_core.vhd
# (a C-preprocessor output, not cpugen either). So those three are extracted
# by basename here and placed at their required earlier positions; only the
# REMAINING lines (decode_table_<kind>.vhd + its _config.vhd + the
# cpu_synth_*_config.vhd + any j1/j4 extra architecture files) are spliced at
# the later position after decode_core.vhd, as before.
[ -f "$GEN/cpu_synth_files.list" ] || { echo "ERROR: $GEN/cpu_synth_files.list missing — run gen_synth_sources.sh (soc_gen) first" >&2; exit 1; }
_CPU_SYNTH_DECODE_PKG=""
_CPU_SYNTH_DECODE=""
_CPU_SYNTH_DECODE_BODY=""
_CPU_SYNTH_REMAINING=()
while IFS= read -r _f; do
  [ -n "$_f" ] || continue
  case "$(basename "$_f")" in
    decode_pkg.vhd)   _CPU_SYNTH_DECODE_PKG="$_f" ;;
    decode.vhd)       _CPU_SYNTH_DECODE="$_f" ;;
    decode_body.vhd)  _CPU_SYNTH_DECODE_BODY="$_f" ;;
    *) _CPU_SYNTH_REMAINING+=("$_f") ;;
  esac
done < "$GEN/cpu_synth_files.list"
if [ -z "$_CPU_SYNTH_DECODE_PKG" ] || [ -z "$_CPU_SYNTH_DECODE" ] || [ -z "$_CPU_SYNTH_DECODE_BODY" ]; then
  echo "ERROR: $GEN/cpu_synth_files.list missing decode_pkg.vhd/decode.vhd/decode_body.vhd — regenerate with soc_gen (make <board> TARGET=soc_gen)" >&2
  exit 1
fi
FILES=(
  $CPU/cpu2j0_pkg.vhd
  $CPU/core/components_pkg.vhd
  # tlb: cpu.vhd directly instantiates work.tlb (entity inst. in the PRIV_ARCH
  # generate), so ghdl needs it analyzed before cpu.vhd for ALL variants.
  $CPU/core/tlb.vhd
  $CPU/core/mult_pkg.vhd
  $CPU/$_CPU_SYNTH_DECODE_PKG
  $CPU/core/datapath_pkg.vhd
  $CPU/core/cpu.vhd
  $CPU/core/mult.vhd
  $CPU/core/datapath.vhd
  $CPU/core/shifter.vhd
  $CPU/core/register_file.vhd
  $CPU/core/register_file_flops.vhd
  $CPU/core/register_file_two_bank.vhd
  $CPU/$_CPU_SYNTH_DECODE
  $CPU/$_CPU_SYNTH_DECODE_BODY
  $CPU/decode/decode_table.vhd
  $CPU/decode/decode_core.vhd
)
# jcore-cpu gained core/divider{,_pkg}.vhd at 998a09c (SH-2A sequential divider).
# Pins older than that do not have them, and naming a nonexistent file makes ghdl
# fail outright -- which would block bisecting the submodule across that boundary.
# Include them only when present, and say so out loud: a silently shrinking file
# list would mean quietly synthesizing a different design than intended.
#
# Order matters: divider_pkg must precede cpu.vhd (which `use`s it) and divider.vhd
# must precede datapath.vhd. Both are inserted by rebuilding FILES around the
# anchors rather than appending, so analyze order is preserved.
_fl_with_divider=()
for _f in "${FILES[@]}"; do
  case "$_f" in
    "$CPU/core/mult_pkg.vhd")
      _fl_with_divider+=("$_f")
      if [ -f "$CPU/core/divider_pkg.vhd" ]; then
        _fl_with_divider+=("$CPU/core/divider_pkg.vhd")
      else
        echo "filelist: skipping absent $CPU/core/divider_pkg.vhd" >&2
      fi
      ;;
    "$CPU/core/mult.vhd")
      _fl_with_divider+=("$_f")
      if [ -f "$CPU/core/divider.vhd" ]; then
        _fl_with_divider+=("$CPU/core/divider.vhd")
      else
        echo "filelist: skipping absent $CPU/core/divider.vhd" >&2
      fi
      ;;
    *) _fl_with_divider+=("$_f") ;;
  esac
done
FILES=("${_fl_with_divider[@]}")
unset _fl_with_divider _f
# Splice in the REMAINING soc_gen-generated variant synth sources (decode
# table + configs; decode_pkg/decode/decode_body were already placed above),
# $CPU-prefixed.
for _f in "${_CPU_SYNTH_REMAINING[@]}"; do
  FILES+=("$CPU/$_f")
done
unset _CPU_SYNTH_DECODE_PKG _CPU_SYNTH_DECODE _CPU_SYNTH_DECODE_BODY _CPU_SYNTH_REMAINING _f
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
  # icache_modereg: the cache-mode register that also hosts the SMP IPI trigger
  # (bit-28 -> per-core int). Single-core variants never instantiated it; the
  # dual variants' generated devices.vhd does (as the socgen `ipi` device), so
  # it must be v2p'd (synth.sh/sim.sh) + analyzed. Deps (cache_pkg/data_bus_pkg/
  # ddrc_cnt_pkg/attr_pkg) are all above. Analyze the soc_port_*-stripped $GEN
  # copy (gen_synth_sources.sh) -- its soc_port_global_name attrs make ghdl
  # --synth assert in the dual netlist (synth-vhdl_decls), same as gpio2 below.
  $GEN/icache_modereg.vhd
  components/cpu/cache/dcache.vhd  # posedge _sc CDC (cache_clkmode_sc); Part B
  components/cpu/cache/icache.vhd
  components/cpu/cache/cache_config_fpga.vhd
  components/misc/bus_mux_typecsub.vhd
  components/misc/bus_mux_typec.vhd
  # dual-core cpus1 bus arbiter (multi_master_bus_mux): devices.vhd
  # instantiates it (cpus_mux) only in dual variants (peripheral-buses.cpu1),
  # but analyzing it unconditionally is harmless for single-core variants.
  components/misc/multi_master_bus_mux.vhd
  targets/boards/ulx3s/generated/ddr_ram_mux.vhd  # soc_gen metadata stripped
  targets/ddr_ram_mux/one_cpu_idcache.vhd
  targets/ddr_ram_mux/one_cpu_idcache_fpga.vhd
  # dual-core ddr_ram_mux configuration (bound by soc.vhd only in dual
  # variants via soc_gen's ddr_ram_mux.configuration); analyzed unconditionally
  # like the one_cpu_* configs above, harmless for single-core variants.
  targets/ddr_ram_mux/two_cpu_idcache.vhd
  targets/ddr_ram_mux/two_cpu_idcache_fpga.vhd
  targets/cpu_core_pkg.vhd
  targets/cpu_core.vhd
  targets/cpus.vhd
  lib/hwutils/data_bus_delay.vhd
  lib/hwutils/instr_bus_delay.vhd
  components/uartlite/uart_pkg.vhd
  components/uartlite/uart.vhd
  # uartlitedb + gpio2 (below) carry an soc_port_irq attribute that ghdl --synth
  # asserts on (synth-vhdl_decls) in the dual-core netlist; gen_synth_sources.sh
  # writes attribute-stripped copies into $GEN that we analyze in their place.
  $GEN/uartlitedb.vhd
  # M2: AIC v1 (interrupt controller + RTC + PIT) + peripheral bus mux
  components/misc/aic2_pkg.vhd
  components/misc/aic_edgedet.vhd
  components/misc/aic.vhd
  $GEN/gpio2.vhd
  $GEN/spi2.vhd                       # soc_port_local_name stripped (see above)
  targets/boards/ulx3s/periph_mux.vhd
  targets/boards/ulx3s/boot_image_pkg.vhd
  components/memory/bootram_infer.vhd
  components/sdram/sdram_pkg.vhd
  components/sdram/sdram_ctrl.vhd
  components/sdram/sdram_iocells.vhd
  targets/boards/ulx3s/ulx3s_clkgen.vhd
  targets/cpumreg.vhd
  targets/boards/ulx3s/cpus_one_m0_arch.vhd
  targets/boards/ulx3s/cpus_two_m0_arch.vhd
  # soc_gen-generated cpus configuration (soc_cpus_config) replacing the retired
  # hand-written one_cpu_m0_direct_fpga; binds cpu_synth_direct for the j2-direct
  # default variant. Must follow the cpus entity + one_cpu_m0 arch + cpu_synth.
  targets/boards/ulx3s/generated/cpus_config.vhd
  # padring entities soc_gen instantiates but does not emit (leaves).
  targets/boards/ulx3s/reset_sync.vhd
  targets/boards/ulx3s/aic_irq_gen.vhd
  # aic_irq_combine: SMP-only leaf that ORs the ipi int0 into aic0's irq_i on
  # the dual-core variants (soc_gen instantiates it in dual-common but does not
  # emit it). Analyzed for all variants (unused/uninstantiated on single-core).
  targets/boards/ulx3s/aic_irq_combine.vhd
  # the soc_gen-generated trio (leaf-first: devices <- soc <- pad_ring), now the
  # synthesized/elaborated board top (replaces the retired hand-written
  # ulx3s_top.vhd). The ECP5 clkgen arch (clkgen(ecp5)/EHXPLLL) that pad_ring
  # binds is appended by the consumer script (real primitive for synth, the
  # tb/ehxpll_sim.vhd stand-in for sim).
  targets/boards/ulx3s/devices.vhd
  targets/boards/ulx3s/soc.vhd
  targets/boards/ulx3s/pad_ring.vhd
)
