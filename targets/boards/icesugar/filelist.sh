# Canonical ghdl analyze order for the iCESugar EBR-only J1 design.
# Sourced by sim.sh and synth.sh (after cd to repo root). Defines FILES.
# The cpu .vhd files must be generated first: `make -C components/cpu/decode
# generate` + v2p of mult/datapath/decode_core and gpio2/uart (done by
# sim.sh/synth.sh). `make icesugar TARGET=soc_gen` must have produced
# cpus_config.vhd + cpu_synth_files.list + devices.vhd + soc.vhd.
CPU=components/cpu
BRD=targets/boards/icesugar
# The J1 variant synth sources (decode_pkg/decode/decode_body + EBR register
# file, sequential mult/shifter, ROM decode table + config, cpu_synth_j1
# config) are soc_gen-generated, one per line, cpu-submodule-relative.
#
# ALL SIX cpugen outputs (decode_pkg.vhd, decode.vhd, decode_body.vhd, and
# the three decode_table_*.vhd) are emitted by ONE cpugen invocation and
# share layout constants (e.g. decode_pkg.vhd's DEC_ADDR_BITS must match the
# selected decode_table_<kind>.vhd's ROM geometry) -- see
# tools/socgen/elaborate/cpumap.go's decodeGenFiles comment. This file used
# to hardcode decode_pkg.vhd/decode.vhd/decode_body.vhd to the committed
# base decode/ tree AND blindly splice the whole cpu_synth_files.list (which
# ALSO carries those same three names, from gen/j1-w72/decode/) -- so each of
# the three was analyzed TWICE, from two different trees, and ghdl picked
# whichever definition it saw last. Fixed the same way ulx3s's and
# gf180_j4mmu's filelist.sh were: extract the three model-invariant names by
# basename and place them at their required earlier positions (decode_pkg.vhd
# before cpu.vhd, which `use`s its types; decode.vhd/decode_body.vhd after
# cpu/mult/datapath but before the static decode_table.vhd/decode_core.vhd),
# and splice only the REMAINING entries (the selected table + its config +
# cpu_synth config + J1's EBR/DSP extra architectures) at the later position.
[ -f "$BRD/cpu_synth_files.list" ] || { echo "ERROR: $BRD/cpu_synth_files.list missing — run make icesugar TARGET=soc_gen first" >&2; exit 1; }
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
done < "$BRD/cpu_synth_files.list"
if [ -z "$_CPU_SYNTH_DECODE_PKG" ] || [ -z "$_CPU_SYNTH_DECODE" ] || [ -z "$_CPU_SYNTH_DECODE_BODY" ]; then
  echo "ERROR: $BRD/cpu_synth_files.list missing decode_pkg.vhd/decode.vhd/decode_body.vhd — regenerate with 'make icesugar TARGET=soc_gen'" >&2
  exit 1
fi
FILES=(
  $CPU/cpu2j0_pkg.vhd
  $CPU/core/components_pkg.vhd
  # tlb: cpu.vhd directly instantiates work.tlb in the PRIV_ARCH generate, so
  # ghdl needs it analyzed before cpu.vhd for all variants.
  $CPU/core/tlb.vhd
  $CPU/core/mult_pkg.vhd
  $CPU/core/divider_pkg.vhd
  $CPU/$_CPU_SYNTH_DECODE_PKG
  $CPU/core/datapath_pkg.vhd
  $CPU/core/cpu.vhd
  $CPU/core/mult.vhd
  $CPU/core/divider.vhd
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
# Splice in the REMAINING soc_gen-generated J1 synth sources, $CPU-prefixed.
for _f in "${_CPU_SYNTH_REMAINING[@]}"; do
  FILES+=("$CPU/$_f")
done
unset _CPU_SYNTH_DECODE_PKG _CPU_SYNTH_DECODE _CPU_SYNTH_DECODE_BODY _CPU_SYNTH_REMAINING _f
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
  components/memory/spram_128k.vhd
  components/memory/dev_ddr_spram.vhd
  # Peripherals served by the generated devices.vhd: uartlite + gpio2 + the
  # multi-master peripheral bus mux.
  components/uartlite/uart_pkg.vhd
  components/uartlite/uart.vhd
  components/uartlite/uartlitedb.vhd
  components/misc/bus_mux_pkg.vhd
  components/misc/multi_master_bus_mux.vhd
  components/misc/gpio2.vhd
  # W5500 Ethernet over SPI (spi device class).
  components/misc/spi2.vhd
  # Free-running 32-bit read-only cycle counter (cycle_counter device class).
  components/misc/cycle_counter.vhd
  # Second, hand-maintained cpus architecture kept analyzable alongside the
  # active one (not referenced by the generated cpus_config.vhd binding).
  $BRD/cpus_one_ebr.vhd
  # cpus_coremark: flash-boot arch (Task 7b), activated via design.yaml's
  # cpu.architecture (Task 8a) -- soc_gen's generated cpus_config.vhd binds
  # this architecture, so its dependencies must analyze before cpus_config.vhd.
  # flash_boot_reader.vhd / ice_spi_io.vhd are pulled in transitively via
  # components/misc/build.mk's file list, but this filelist.sh is
  # hand-maintained (not build.mk-driven), so list them explicitly.
  components/misc/flash_boot_reader.vhd
  components/misc/ice_spi_io.vhd
  components/memory/dev_ddr_spram_boot.vhd
  $BRD/boot_image_coremark_pkg.vhd
  components/memory/bootram_infer_coremark.vhd
  $BRD/cpus_coremark.vhd
  $BRD/cpus_coremark_config.vhd
  # soc_gen-generated SoC: cpus_config.vhd binds cpus_coremark to
  # cpu_synth_j1_dsp; devices.vhd precedes soc.vhd.
  $BRD/cpus_config.vhd
  $BRD/devices.vhd
  $BRD/soc.vhd
  # Board top (soc_gen-generated pad_ring) + 12 MHz clkgen.
  $BRD/ice_clkgen.vhd
  $BRD/pad_ring.vhd
)
