library ieee;
use ieee.std_logic_1164.all;

-- iCESugar clock/reset: the J1 runs directly at the 12 MHz oscillator rate
-- (no SB_PLL40). clk_out is clk_in; rst_out is a power-on reset held for a few
-- cycles then released, synchronized to clk_in. Kept as a separate entity so a
-- later PLL can drop in without touching icesugar_top.
entity ice_clkgen is
  port (
    clk_in  : in  std_logic;
    clk_out : out std_logic;
    rst_out : out std_logic);
end entity;

architecture rtl of ice_clkgen is
  signal por : std_logic_vector(3 downto 0) := (others => '1');
begin
  clk_out <= clk_in;
  process (clk_in)
  begin
    if rising_edge(clk_in) then
      por <= por(2 downto 0) & '0';
    end if;
  end process;
  rst_out <= por(3);
end architecture;
