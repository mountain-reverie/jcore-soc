-- Elaboration-only top for the GF180 area harden of boot_mem: a
-- fixed-generic wrapper around bootram_infer(boot_mem_gf180) (see
-- lib/memory_tech_lib/tech/gf180/boot_mem_stack_gf180.vhd), used ONLY by
-- targets/asic/gf180_j4mmu/librelane/boot_mem/ (run.sh macro=boot_mem)'s
-- ghdl-yosys netlist generation. `ghdl -e` needs a top with no unbound
-- generics; bootram_infer's c_addr_width defaults to 14 (the base/inferred
-- 16 KiB variant), so this wrapper pins it to 12 (boot_mem's 4 KiB window)
-- and explicitly binds the (boot_mem_gf180) architecture.
--
-- SCRATCHPAD REMOVAL: (boot_mem_gf180) is now PURE READ-ONLY ROM (no
-- vendor SRAM macro at all -- the writable stack lane it used to back
-- with 4x gf180mcu_fd_ip_sram__sram512x8m8wm1 macros is gone; SDRAM
-- backs the boot-time stack instead, see boot_image_pkg.vhd). The
-- LibreLane config for this macro (librelane/boot_mem/config.json) must
-- NOT declare any MACROS/fixed DIE_AREA any more -- see that file.
library ieee;
use ieee.std_logic_1164.all;
use work.cpu2j0_pack.all;

entity boot_mem_top_gf180 is
  port (
    clk    : in  std_logic;
    ibus_i : in  cpu_instruction_o_t;
    ibus_o : out cpu_instruction_i_t;
    db_i   : in  cpu_data_o_t;
    db_o   : out cpu_data_i_t);
end entity;

architecture top of boot_mem_top_gf180 is
begin
  u_boot_mem : entity work.bootram_infer(boot_mem_gf180)
    generic map (c_addr_width => 12)
    port map (
      clk    => clk,
      ibus_i => ibus_i,
      ibus_o => ibus_o,
      db_i   => db_i,
      db_o   => db_o);
end architecture;
