#!/usr/bin/env bash
# Build the EBR boot image, analyze the iCESugar EBR-only J1 design under ghdl,
# and run the top-level banner testbench (drive 12 MHz, decode ser_tx, assert
# the boot banner). Full nextpnr synthesis is synth.sh.
#
# `bash sim.sh xip` instead runs the Task-7 end-to-end XIP demand-paging
# cosim (cpus(one_cpu_xip) + a behavioral SPI flash preloaded with
# xip_test.img, see tb/cpus_xip_tb.vhd) -- a much smaller, faster build than
# the full board top (no devices.vhd/soc.vhd/pad_ring, no uart/gpio2/aic/
# spi2), so it is handled as an early, separate branch below.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../../.." && pwd)"
WORK="${WORK:-/tmp/icework}"
cd "$ROOT"
rm -rf "$WORK"; mkdir -p "$WORK"

if [ "${1:-}" = "xip" ]; then
  # 0x. boot image: pack the XIP resident set (reset vectors + crt0 +
  #     _pf_handler + round-robin victim counter, Task 6's xip_handler.s)
  #     into the SAME bootram_infer init package name (boot_image_pkg.vhd)
  #     cpus_one_ebr's own boot flow regenerates above -- bootram_infer.vhd
  #     hardcodes `use work.boot_image_pkg.all`, so there is only ever one
  #     "current" boot image on disk; this branch's own fresh `rm -rf $WORK`
  #     + regeneration means it never collides with a stale one from a prior
  #     run of the other mode.
  make -C targets/boards/icesugar/rom xip_test.elf xip_test.img xip_boot.bin
  perl tools/genbootpkg \
      targets/boards/icesugar/rom/xip_boot.bin \
      512 \
      > targets/boards/icesugar/boot_image_pkg.vhd

  # 0y. flash image: pack xip_test.img (the >4-page test program + table,
  #     linked at LMA/flash-offset 0x100000) into a byte-array VHDL package
  #     the tb's behavioral flash model serves from. Padded to 6 pages
  #     (24576 B) -- xip_test.img is 20052 B (4.9 pages); the pad is
  #     zero-filled and never read by the test program.
  perl tools/genflashpkg \
      targets/boards/icesugar/rom/xip_test.img \
      "16#100000#" \
      24576 \
      > targets/boards/icesugar/xip_flash_pkg.vhd

  # 1. generated cpu sources (same prerequisite as the full board build --
  #    cpus_xip.vhd's core0 still needs the decode tables + v2p'd cores).
  make -C components/cpu/decode generate
  ( cd components/cpu && for f in core/mult core/datapath decode/decode_core; do
      LD_LIBRARY_PATH='' perl ../../tools/v2p < "$f.vhm" > "$f.vhd"; done )

  # 2. soc_gen: regenerate cpus_config.vhd / cpu_synth_files.list / the
  #    generated `cpus`/cpu_core/clk_config/data_bus_pkg targets/*.vhd
  #    cpus_xip.vhd binds against. (devices.vhd/soc.vhd/pad_ring are also
  #    regenerated as a side effect but unused by this branch.)
  make icesugar TARGET=soc_gen

  # 3. analyze the reduced XIP design: the same CPU-core + soc_gen-generated
  #    prefix as filelist.sh (through dev_ddr_spram.vhd), then the spi_page_cache
  #    sub-project's own sources (Tasks 1-5) + the generated xip_flash_pkg +
  #    cpus_xip.vhd (Task 5) -- NOT the uart/gpio2/spi2/aic/devices.vhd/soc.vhd/
  #    pad_ring chain (cpus_one_ebr-specific, unused by cpus_xip's arch).
  CPU=components/cpu
  BRD=targets/boards/icesugar
  [ -f "$BRD/cpu_synth_files.list" ] || { echo "ERROR: $BRD/cpu_synth_files.list missing" >&2; exit 1; }
  FILES=(
    $CPU/cpu2j0_pkg.vhd
    $CPU/core/components_pkg.vhd
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
  while IFS= read -r _f; do
    [ -n "$_f" ] && FILES+=("$CPU/$_f")
  done < "$BRD/cpu_synth_files.list"
  FILES+=(
    lib/hwutils/attr_pkg.vhd
    components/misc/misc_pkg.vhd
    output/icesugar/config/config.vhd
    targets/clk_config.vhd
    targets/data_bus_pkg.vhd
    targets/cpu_core_pkg.vhd
    targets/cpu_core.vhd
    targets/cpus.vhd
    $BRD/boot_image_pkg.vhd
    components/memory/bootram_infer.vhd
    components/memory/spram_128k.vhd
    components/memory/dev_ddr_spram.vhd
    # spi_page_cache sub-project (Tasks 1-4): fill engine, MMIO/window pack,
    # the page cache itself, the config-flash SB_IO pad wrapper.
    components/misc/spi_page_cache_pkg.vhd
    components/misc/spi_flash_fill.vhd
    components/misc/spi_page_cache.vhd
    components/misc/ice_spi_io.vhd
    # sb_io_sim.vhd: sim-only behavioral SB_IO model (tristate), needed to
    # bind ice_spi_io's unbound SB_IO instances (--syn-binding maps them to
    # this architecture; synth leaves them for yosys to map to the real cell).
    components/emac/sb_io_sim.vhd
    # sb_mac16_sim.vhd: sim-only behavioral iCE40 SB_MAC16 DSP model. The
    # j1_dsp CPU variant (cpu_synth_j1_dsp, bound by one_cpu_xip_core_cfg)
    # maps its ALU add/sub onto an SB_MAC16 DSP block (core/dsp_arith.vhd).
    # Under --syn-binding the SB_MAC16 instances are left unbound unless this
    # behavioral model is analyzed, so WITHOUT it every arith_out is 'X' ->
    # all pc-relative load addresses are 'X' -> the CPU never reaches the XIP
    # window and the cosim times out. (The full-board build adds this same
    # file for exactly this reason; the xip file list must too.)
    components/cpu/core/sb_mac16_sim.vhd
    # Generated flash-image package (this branch's step 0y above).
    $BRD/xip_flash_pkg.vhd
    # Task 5 arch: cpus(one_cpu_xip).
    $BRD/cpus_xip.vhd
  )
  # NOTE: --std=08 here (not 93) is load-bearing, not cosmetic: GHDL keys its
  # work library file by std version (work-obj93.cf vs work-obj08.cf) inside
  # a shared --workdir, so units analyzed under one std are invisible to a
  # `use work.foo.all` in a file analyzed under a different std -- they are
  # two separate physical libraries that happen to share a directory. Since
  # Step 4 below MUST use --std=08 (VHDL-2008 external names), this array
  # analyzes under --std=08 too so the tb's `use work.cpu2j0_pack.all` etc.
  # can actually see these units. (GHDL 08 accepts this std=93-era design
  # unchanged; only the tb needs 2008 syntax.)
  ghdl -a --std=08 -fexplicit -fsynopsys -C --workdir="$WORK" "${FILES[@]}"

  # 4. Task 7 cosim tb: VHDL-2008 external names reach cpus_xip.vhd's internal
  #    pad-level SPI pins (pin_cs_n/sck/mosi/miso) and the pre-decode
  #    data_master_snoop tap -- neither is a port on the generated `cpus`
  #    entity (see the tb's header comment) -- so ONLY this file is analyzed
  #    under --std=08; everything above stays --std=93 (external names work
  #    across std versions; only the referencing unit needs 2008 syntax).
  echo "=== cpus_xip_tb (Task 7 end-to-end XIP paging cosim) ==="
  ghdl -a --std=08 -fexplicit -fsynopsys -C --workdir="$WORK" \
      targets/boards/icesugar/tb/cpus_xip_tb.vhd
  ghdl -e --std=08 -fexplicit -fsynopsys -C --syn-binding --workdir="$WORK" cpus_xip_tb
  ghdl -r --std=08 -fexplicit -fsynopsys -C --syn-binding --workdir="$WORK" cpus_xip_tb \
      --stop-time=65ms --assert-level=error ${XIP_VCD:+--vcd=$XIP_VCD}
  exit 0
fi

# 0. boot image: cross-compile the standalone banner/blink program (its own
#    crt0 + linker script, no boot/ submodule), then pack the SREC/bin into the
#    bootram_infer init package (c_addr_width = 11 -> 512 words). This
#    overwrites the boot_image_pkg.vhd placeholder with the real program.
make -C targets/boards/icesugar/rom all
perl tools/genbootpkg \
    targets/boards/icesugar/rom/boot.bin \
    512 \
    > targets/boards/icesugar/boot_image_pkg.vhd

# 1. generated cpu sources: decode tables (generate) + v2p of the templated
#    cores, plus the v2p'd uart / gpio2 peripherals. Must precede soc_gen so the
#    VHDL library it parses is complete (otherwise it emits a degenerate soc).
make -C components/cpu/decode generate
( cd components/cpu && for f in core/mult core/datapath decode/decode_core; do
    LD_LIBRARY_PATH='' perl ../../tools/v2p < "$f.vhm" > "$f.vhd"; done )
LD_LIBRARY_PATH='' perl tools/v2p < components/uartlite/uart.vhm > components/uartlite/uart.vhd
LD_LIBRARY_PATH='' perl tools/v2p < components/misc/gpio2.vhm > components/misc/gpio2.vhd

# 2. soc_gen: regenerate devices.vhd / soc.vhd / cpus_config.vhd /
#    cpu_synth_files.list / icesugar.pcf from design.yaml.
make icesugar TARGET=soc_gen

# 3. analyze the full design and elaborate the board top.
source targets/boards/icesugar/filelist.sh   # defines FILES=( ... )
# SB_MAC16 behavioral model: sim-only stand-in for the iCE40 DSP that
# mult(ice40dsp) instantiates. Synthesis (synth.sh) leaves SB_MAC16 an unbound
# component for yosys to map to the real cell, so this is added ONLY here.
# -C (--mb-comments) allows the multi-byte Unicode arrows/quotes used in
# sb_mac16_sim.vhd's comments; current GHDL (6.0.0) rejects them under
# --std=93 without it, on every ghdl invocation (analyze/elaborate/run) that
# touches this file.
# sb_io_sim.vhd: sim-only iCE40 SB_IO model (tristate/open-drain), needed to
# bind ice_i2c_io's unbound SB_IO instances for the bit-banged DS3231 I2C.
# ds3231_model.vhd: sim-only behavioral I2C slave (the DS3231 RTC) hooked
# onto pad_ring's pin_i2c_scl/pin_i2c_sda in the testbench below. Neither is
# in the synth filelist -- synthesis maps SB_IO to the real cell and there is
# no DS3231 model to synthesize.
FILES=( components/cpu/core/sb_mac16_sim.vhd components/memory/sb_spram256ka_sim.vhd components/emac/sb_pll40_2_pad_sim.vhd components/emac/sb_io_sim.vhd components/emac/w5500_model.vhd components/misc/ds3231_model.vhd "${FILES[@]}" )
ghdl -a --std=93 -fexplicit -fsynopsys -C --workdir="$WORK" "${FILES[@]}"
ghdl -e --std=93 -fexplicit -fsynopsys -C --syn-binding --workdir="$WORK" pad_ring
echo "pad_ring elaborated OK"

# 4. top-level banner testbench: drive 12 MHz, decode ser_tx, assert the banner.
echo "=== icesugar_top_tb ==="
ghdl -a --std=93 -fexplicit -fsynopsys -C --workdir="$WORK" \
    targets/boards/icesugar/tb/icesugar_top_tb.vhd
ghdl -e --std=93 -fexplicit -fsynopsys -C --syn-binding --workdir="$WORK" icesugar_top_tb
ghdl -r --std=93 -fexplicit -fsynopsys -C --syn-binding --workdir="$WORK" icesugar_top_tb \
    --stop-time=210ms --assert-level=error
