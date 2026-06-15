library ieee;
use ieee.std_logic_1164.all;

-- Tech-abstracted clock generator. The entity is the ASIC PLL seam. The
-- architecture is selected by which file the build analyzes (no configuration):
-- the sim flow analyzes this file's `sim` arch; the synth flow also analyzes
-- ulx3s_clkgen_ecp5.vhd (its `ecp5` arch, analyzed last, wins default binding).
entity clkgen is
  port (
    clk_in : in  std_logic;   -- 25 MHz board oscillator
    rst_in : in  std_logic;   -- async external reset (active high)
    clk    : out std_logic;   -- CPU clock
    locked : out std_logic);  -- high when clk is stable
end entity;

-- Simulation: passthrough, lock after a couple cycles.
architecture sim of clkgen is
begin
  clk <= clk_in;
  process(clk_in, rst_in)
    variable cnt : integer := 0;
  begin
    if rst_in = '1' then
      locked <= '0'; cnt := 0;
    elsif rising_edge(clk_in) then
      if cnt < 4 then cnt := cnt + 1; locked <= '0';
      else locked <= '1'; end if;
    end if;
  end process;
end architecture;
