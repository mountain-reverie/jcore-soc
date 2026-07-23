library ieee;
use ieee.std_logic_1164.all;

-- Task 3 (QSPI XIP sub-project) boot ROM: a tiny jump-to-flash stub,
-- replacing the Task 1 all-zero scaffold. Reset PC on this core is
-- hard-wired to address 0 (targets/cpu_core_pack's DEC_CORE_RESET microcode
-- entry point ends up fetching user instructions starting at 0x00000000,
-- decode_core_instr_addr's addr[31:28]=0x0 -> DEV_SRAM, i.e. this ROM,
-- serviced by components/memory/bootram_infer.vhd) -- there is no SH
-- vector-table indirection (no PC/SP longs at 0/4); the first fetched
-- instruction IS the reset entry point.
--
-- NOTE: targets/asic/gf180_j4mmu/sim/rtl.sh (this target's own banner/
-- SPI-loopback self-test) OVERWRITES this file with a real ulx3s-style
-- bootloader image (via tools/genbootpkg) as one of its build steps -- it
-- treats boot_image_pkg.vhd as a build product for that specific test, NOT
-- as the source of truth. This hand-authored version IS the source of truth
-- for plain `ghdl -e soc` elaboration (README.md) and for the flash-XIP
-- boot chain Task 4 builds on; if you run sim/rtl.sh, restore this file
-- (e.g. `git checkout`) afterward.
--
-- Contents (assembled with sh2-elf-as from):
--   mov.l   1f, r0      ! r0 = flash XIP entry (see design.flash.yaml,
--                       ! Task 3: flash_base = 0x14000000, chosen so the
--                       ! CPU-core address decode routes it to DEV_DDR and
--                       ! it is genuinely fetchable through the icache)
--   jmp     @r0
--   nop                 ! delay slot
--   .align 2
-- 1:
--   .long   0x14000000
--
-- Raw big-endian bytes: d0 01 40 2b 00 09 00 09 14 00 00 00 -- packed 4
-- bytes/word (bootram_infer.vhd reads 32-bit words and selects the high or
-- low 16-bit half per db_i.a(1)/ibus_i.a(1)):
--   word 0 (byte addr 0x0): 0xd001402b = mov.l @(disp,PC),r0 ; jmp @r0
--   word 1 (byte addr 0x4): 0x00090009 = nop (delay slot) ; nop (align pad)
--   word 2 (byte addr 0x8): 0x14000000 = the 32-bit literal loaded by mov.l
--     (PC-relative disp=1 from the mov.l at 0x0: (0x0+4)&~3 + 1*4 = 0x8)
--
-- reset -> ROM (this file, address 0x00000000) -> DEV_DDR fetch/access at
-- 0x14000000 (mem_region_mux flash region, design.flash.yaml) is the
-- address chain Task 4's cosim must reproduce.
package boot_image_pkg is
  constant BOOT_DEPTH : integer := 4096;
  type boot_image_t is array (0 to 4095) of std_logic_vector(31 downto 0);
  constant BOOT_IMAGE : boot_image_t := (
    0 => x"d001402b",
    1 => x"00090009",
    2 => x"14000000",
    others => x"00000000");
end package boot_image_pkg;
