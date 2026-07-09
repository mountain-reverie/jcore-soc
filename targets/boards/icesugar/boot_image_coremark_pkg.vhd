library ieee;
use ieee.std_logic_1164.all;

-- boot_image_coremark_pkg: minimal boot EBR image for the cpus_coremark
-- architecture. Unlike boot_image_pkg.vhd (which holds real startup code
-- run directly out of the boot EBR, used by the banner board's one_cpu_ebr
-- arch), this image is JUST a vector table: the payload lives in SPRAM
-- (loaded from config flash by flash_boot_reader before the CPU is released
-- from reset), so all the boot EBR needs to do is point the SH-2 reset PC/SP
-- at it. Confirmed reset semantics (see components/memory/bootram_infer.vhd
-- + targets/boards/icesugar/rom/start.S + boot_image_pkg.vhd): word 0 =
-- power-on PC, word 1 = power-on SP, word 2 = manual-reset PC, word 3 =
-- manual-reset SP -- no stub code required, since the payload's own crt0
-- (e.g. rom/start.S-equivalent linked into the SPRAM image) sets its own SP
-- as its first act; the vector-table SP value is provided anyway so the
-- manual-reset vectors are well-formed.
--
-- word 0/2 = 0x10000000 (PC -> base of SPRAM, DEV_DDR region)
-- word 1/3 = 0x10020000 (SP -> top of 128 KiB SPRAM)
-- Does NOT touch/replace boot_image_pkg.vhd, which the banner board still
-- uses.
package boot_image_coremark_pkg is
  constant BOOT_DEPTH : integer := 512;
  type boot_image_t is array (0 to 511) of std_logic_vector(31 downto 0);
  constant BOOT_IMAGE : boot_image_t := (
    0 => x"10000000",
    1 => x"10020000",
    2 => x"10000000",
    3 => x"10020000",
    others => x"00000000");
end package boot_image_coremark_pkg;
