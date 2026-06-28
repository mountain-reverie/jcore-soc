library ieee;
use ieee.std_logic_1164.all;

-- Placeholder boot image for the iCESugar EBR boot RAM (bootram_infer,
-- c_addr_width = 13 -> 2048 words). This hand-written stub lets the SoC
-- elaborate and synthesise without a cross-compiler; the real boot program is
-- produced by the board ROM build (genbootpkg) in synth.sh / sim.sh, which
-- overwrites this package. Word 0 is a SuperH "bra ." self-branch (0xaffe)
-- followed by nop (0x0009) so a powered-on core spins safely rather than
-- running uninitialised memory.
package boot_image_pkg is
  constant BOOT_DEPTH : integer := 2048;
  type boot_image_t is array (0 to 2047) of std_logic_vector(31 downto 0);
  constant BOOT_IMAGE : boot_image_t := (
    0 => x"affe0009",
    others => x"00000000");
end package;
