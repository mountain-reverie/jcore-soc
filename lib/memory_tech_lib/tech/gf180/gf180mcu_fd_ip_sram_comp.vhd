-- Black-box component declaration for the GlobalFoundries GF180MCU hard-IP
-- single-port synchronous SRAM macro `gf180mcu_fd_ip_sram__sram256x8m8wm1`
-- (256 words x 8 bits, per-bit write mask). No architecture body is
-- provided here -- the vendor macro is supplied out-of-tree at synthesis/
-- P&R time from $PDK_ROOT/gf180mcuD/libs.ref/gf180mcu_fd_ip_sram/ (verilog
-- blackbox / .lib / .lef / .gds). This package only exists so the
-- tech/gf180 VHDL architecture below has something to instantiate and so a
-- yosys `-e`/`blackbox` pass has a matching module name to keep un-touched.
--
-- Vendor port semantics (confirmed from the behavioral
-- gf180mcu_fd_ip_sram__sram256x8m8wm1.v):
--   CLK   : in  -- clock
--   CEN   : in  -- chip enable,        ACTIVE-LOW  (0 = chip selected)
--   GWEN  : in  -- global write enable, ACTIVE-LOW (0 = write, 1 = read)
--   WEN   : in (7:0) -- per-bit write mask, ACTIVE-LOW (0 = write that bit)
--   A     : in (7:0) -- address, 256 deep
--   D     : in (7:0) -- write data
--   Q     : out(7:0) -- read data, REGISTERED (1-cycle sync read, same as
--                        jcore's ram_1rw/tech-sim semantics -- no adapter
--                        needed)
--   VDD/VSS : supplies (left unconnected/open here; tied by the P&R flow)
library ieee;
use ieee.std_logic_1164.all;

package gf180_sram_comp_pkg is

  -- VHDL-93 basic identifiers can't contain consecutive underscores, so the
  -- vendor's exact cell name is used here via extended-identifier syntax
  -- (\...\) -- this is still the literal name `gf180mcu_fd_ip_sram__sram256x8m8wm1`
  -- as far as elaboration/yosys module naming is concerned.
  component \gf180mcu_fd_ip_sram__sram256x8m8wm1\ is
    port (
      CLK  : in  std_logic;
      CEN  : in  std_logic;
      GWEN : in  std_logic;
      WEN  : in  std_logic_vector(7 downto 0);
      A    : in  std_logic_vector(7 downto 0);
      D    : in  std_logic_vector(7 downto 0);
      Q    : out std_logic_vector(7 downto 0));
  end component;

  -- 512-deep x 8-bit sibling macro (same port semantics/polarities as the
  -- 256x8 variant above, just A(8:0) instead of A(7:0)). Used by the cache
  -- DATA RAM wrapper (ram_2x8x2048_2rw_gf180.vhd), which tiles 8 of these
  -- (4 row-banks x 2 byte-columns) to cover 2048-deep x 16-bit.
  component \gf180mcu_fd_ip_sram__sram512x8m8wm1\ is
    port (
      CLK  : in  std_logic;
      CEN  : in  std_logic;
      GWEN : in  std_logic;
      WEN  : in  std_logic_vector(7 downto 0);
      A    : in  std_logic_vector(8 downto 0);
      D    : in  std_logic_vector(7 downto 0);
      Q    : out std_logic_vector(7 downto 0));
  end component;

end package gf180_sram_comp_pkg;
