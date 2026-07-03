-- Sim-only behavioural model of the iCE40 SB_PLL40_CORE PLL hard block.
-- Instantiated as a COMPONENT by eth_tx; at synthesis it is left unbound
-- (--syn-binding) so yosys synth_ice40 maps the real hard block. EXCLUDED
-- from every synth filelist, exactly like components/cpu/core/sb_mac16_sim.vhd
-- and components/memory/sb_spram256ka_sim.vhd.
--
-- This model ignores REFERENCECLK and simply free-runs a 20 MHz clock on
-- PLLOUTGLOBAL/PLLOUTCORE (25 ns half-period), asserting LOCK after a few us.
-- The DIVR/DIVF/DIVQ/FILTER_RANGE and string generics are accepted and ignored.
library ieee;
use ieee.std_logic_1164.all;

entity SB_PLL40_CORE is
  generic (
    FEEDBACK_PATH : string := "SIMPLE";
    PLLOUT_SELECT : string := "GENCLK";
    DIVR          : std_logic_vector(3 downto 0) := "0000";
    DIVF          : std_logic_vector(6 downto 0) := "0000000";
    DIVQ          : std_logic_vector(2 downto 0) := "000";
    FILTER_RANGE  : std_logic_vector(2 downto 0) := "000");
  port (
    REFERENCECLK : in  std_logic;
    PLLOUTCORE   : out std_logic;
    PLLOUTGLOBAL : out std_logic;
    RESETB       : in  std_logic;
    BYPASS       : in  std_logic;
    LOCK         : out std_logic);
end entity;

architecture behave of SB_PLL40_CORE is
  signal clk20 : std_logic := '0';
begin
  -- Free-running 20 MHz (25 ns half period), independent of REFERENCECLK.
  gen: process
  begin
    clk20 <= '0';
    wait for 25 ns;
    clk20 <= '1';
    wait for 25 ns;
  end process;

  PLLOUTGLOBAL <= clk20;
  PLLOUTCORE   <= clk20;

  lock_proc: process
  begin
    LOCK <= '0';
    wait for 5 us;
    LOCK <= '1';
    wait;
  end process;
end architecture;
