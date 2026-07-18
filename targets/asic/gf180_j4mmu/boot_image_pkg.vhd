library ieee;
use ieee.std_logic_1164.all;

-- Placeholder boot image for GHDL analyze+elaborate only (Task 1 scaffold):
-- an all-zero ROM, NOT a working bootloader. Real boot code (built like
-- targets/boards/ulx3s/boot_image_pkg.vhd via boot/main.c + genram) is out of
-- scope for this task -- see targets/asic/gf180_j4mmu/README.md. Task 2 (sim
-- harness) should replace this with a real boot image if simulation needs one
-- to run past reset.
package boot_image_pkg is
  constant BOOT_DEPTH : integer := 4096;
  type boot_image_t is array (0 to 4095) of std_logic_vector(31 downto 0);
  constant BOOT_IMAGE : boot_image_t := (others => x"00000000");
end package boot_image_pkg;
