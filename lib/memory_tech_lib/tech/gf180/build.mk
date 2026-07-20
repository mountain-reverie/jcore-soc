# tech/gf180: GF180 vendor-hard-IP backend for the memory_tech_lib macros.
# SYNTH/METRICS-ONLY -- these files are not part of the GHDL functional-sim
# path (tech/sim stays the sim backend; see rtl.sh). Included by a synth-only
# build list (e.g. the gf180_j4mmu ASIC target's synth source generation),
# never by build.mk/build_fpga.mk.
#
# Covers the cache tag macro (ram_2x8x256_1rw) and the cache data macro
# (ram_2x8x2048_2rw); other memory_pack macros (ram_18x2048_1rw,
# ram_32x1x512_2rw, rom_32x2048_1r) do not yet have a tech/gf180 backend.

$(VHDLS) += tech/gf180/gf180mcu_fd_ip_sram_comp.vhd
$(VHDLS) += tech/gf180/ram_2x8x256_1rw_gf180.vhd
$(VHDLS) += tech/gf180/ram_2x8x2048_2rw_gf180.vhd
