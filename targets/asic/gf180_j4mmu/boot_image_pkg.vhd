library ieee;
use ieee.std_logic_1164.all;

-- Task 3 (QSPI XIP sub-project) boot ROM: an SH-2 reset VECTOR TABLE that
-- redirects the CPU straight into flash XIP, replacing the Task 1 all-zero
-- scaffold.
--
-- FIX (this file previously carried an executable mov.l/jmp instruction
-- stub at word0/word1 -- that was WRONG. This core's reset is standard SH-2
-- vector-table load, NOT execute-from-address-0: on reset the CPU reads its
-- initial PC from address 0x0 and its initial SP from address 0x4 AS DATA
-- (words, not instructions), then jumps to that PC. It does not fetch and
-- execute the instruction physically stored at 0x0. See
-- targets/boards/ulx3s/rom/start.S:1-8 (the ".vectors" section: `.long
-- _start` at 0x0 = PC, `.long <sp>` at 0x4 = SP, duplicated at 0x8/0xc for
-- manual reset) and targets/boards/ulx3s/rom/ulx3s.ld:9-10, plus the real
-- generated targets/boards/ulx3s/boot_image_pkg.vhd and
-- targets/boards/icesugar/boot_image_pkg.vhd, both of which follow this
-- word0=PC/word1=SP/word2=PC/word3=SP layout with actual code starting only
-- at the PC offset, never at word0/1 themselves.
--
-- Format: this ROM is serviced by components/memory/bootram_infer.vhd
-- (targets/boards/ulx3s/cpus_one_m0_arch.vhd instantiates it with
-- c_addr_width=14, i.e. a 16 KiB, 4096-word boot RAM at byte addresses
-- 0x0000-0x3FFF -- the same instantiation this gf180_j4mmu target reuses,
-- see targets/asic/gf180_j4mmu/filelist.sh). decode_core_instr_addr's
-- addr[31:28]=0x0 -> DEV_SRAM routes fetches here.
--
-- Contents (word-addressed, 4 bytes/word):
--   word 0 (byte addr 0x0): initial PC = 0x14000000, the flash XIP entry
--     (design.flash.yaml, Task 3: flash_base = 0x14000000, chosen so the
--     CPU-core address decode routes it to DEV_DDR -> mem_region_mux ->
--     qspi_flash_ctrl, and it is genuinely fetchable through the icache).
--   word 1 (byte addr 0x4): initial SP = 0x00003ffc, top of this target's
--     16 KiB boot RAM (0x0000-0x3FFF, the *only* memory guaranteed live at
--     reset -- SDRAM/DDR is not yet initialised and flash XIP has no writable
--     backing store). Mirrors targets/boards/ulx3s/rom/start.S's own
--     sp_init/vector-table SP (0x00003ffc, "top of 16 KiB boot RAM"), which
--     is the value validated against this exact bootram_infer(c_addr_width
--     => 14) instantiation. (The *different* 0x00007ffc seen in the
--     committed targets/boards/ulx3s/boot_image_pkg.vhd belongs to a
--     separate, larger real bootloader image built by that target's own
--     sim/rtl.sh step, not to this minimal vector table -- not applicable
--     here.)
--   word 2 (byte addr 0x8): duplicate PC (0x14000000) -- the SH-2 "manual
--     reset" vector, mirroring start.S's _vectors duplication at 0x08/0x0c.
--   word 3 (byte addr 0xc): duplicate SP (0x00003ffc), same rationale.
--
-- reset -> CPU loads PC=word0=0x14000000, SP=word1=0x00003ffc from this ROM
-- -> first instruction FETCH at 0x14000000 -> DEV_DDR -> mem_region_mux ->
-- qspi_flash_ctrl is the address chain Task 4's cosim must reproduce.
--
-- NOTE: targets/asic/gf180_j4mmu/sim/rtl.sh (this target's own banner/
-- SPI-loopback self-test) OVERWRITES this file with a real ulx3s-style
-- bootloader image (via tools/genbootpkg) as one of its build steps, and
-- (as of the QSPI-XIP pre-merge hardening pass) restores this committed
-- version afterward via a save/restore trap, the same convention
-- sim/xip_sim.sh uses for the generated devices.vhd/soc.vhd/.../board.dts
-- -- see rtl.sh's own header.
--
-- *** BASE (NO-FLASH) VARIANT WARNING -- READ BEFORE ANY BASE SYNTH/TAPEOUT ***
-- The committed contents above are the FLASH-VARIANT reset vector table
-- (PC=0x14000000). That address is only fetchable in the FLASH variant,
-- where design.flash.yaml's flash_base=0x14000000 routes it through
-- DEV_DDR -> mem_region_mux -> qspi_flash_ctrl. In the BASE (no-flash)
-- gf180_j4mmu variant there is no qspi_flash_ctrl at all: the same address
-- would route through DEV_DDR -> mem_region_mux -> sdram_ctrl, i.e. an
-- uninitialised/garbage SDRAM fetch -- the CPU would MISBOOT.
-- targets/asic/gf180_j4mmu/prep_sources.sh (shared by both sim/rtl.sh and
-- metrics/synth_gf180.sh) explicitly does NOT regenerate this file ("the
-- boot-ROM image build stays in rtl.sh -- it's sim-only, not needed for
-- synth"), so metrics/synth_gf180.sh's BASE-variant synth-only area
-- metrics currently consume this FLASH vector table verbatim via
-- filelist.sh. That is harmless for synth-only area/stat numbers (no
-- simulation/boot actually happens there), but ANY real base (no-flash)
-- tapeout/signoff flow MUST regenerate a base-appropriate boot image first
-- (e.g. via tools/genbootpkg, exactly as sim/rtl.sh does for its own sim)
-- rather than consuming this committed file as-is. Do not let a future
-- base tapeout script skip this step.
package boot_image_pkg is
  constant BOOT_DEPTH : integer := 4096;
  type boot_image_t is array (0 to 4095) of std_logic_vector(31 downto 0);
  constant BOOT_IMAGE : boot_image_t := (
    0 => x"14000000",
    1 => x"00003ffc",
    2 => x"14000000",
    3 => x"00003ffc",
    others => x"00000000");
end package boot_image_pkg;
