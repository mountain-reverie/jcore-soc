-- Sim-only behavioural model of the iCE40 SB_PLL40_2_PAD PLL hard block.
-- Instantiated as an unbound COMPONENT by ice_clkgen (--syn-binding) so at
-- synthesis yosys synth_ice40 maps the real hard block. EXCLUDED from every
-- synth filelist, exactly like components/cpu/core/sb_mac16_sim.vhd and
-- components/memory/sb_spram256ka_sim.vhd.
--
-- SB_PLL40_2_PAD takes PACKAGEPIN as its dedicated input pad and provides two
-- independent output clocks:
--   PLLOUTGLOBALA / PLLOUTCOREA : fixed reference passthrough (port A has no
--     PLLOUT_SELECT — it always mirrors PACKAGEPIN), used here as the 12 MHz
--     CPU clock, unchanged from the plain passthrough this replaces.
--   PLLOUTGLOBALB / PLLOUTCOREB : PLLOUT_SELECT_PORTB-configured PLL output,
--     used here as the ~20 MHz Ethernet PHY clock. This model free-runs an
--     independent ~20 MHz clock (25 ns half-period) rather than actually
--     multiplying PACKAGEPIN, same simplification as the old
--     sb_pll40_core_sim.vhd it replaces.
-- LOCK asserts a few us after reset. The DIVR/DIVF/DIVQ/FILTER_RANGE and
-- PLLOUT_SELECT_PORTB generics are accepted and ignored.
library ieee;
use ieee.std_logic_1164.all;

entity SB_PLL40_2_PAD is
  generic (
    DIVR               : std_logic_vector(3 downto 0) := "0000";
    DIVF               : std_logic_vector(6 downto 0) := "0000000";
    DIVQ               : std_logic_vector(2 downto 0) := "000";
    FILTER_RANGE       : std_logic_vector(2 downto 0) := "000";
    FEEDBACK_PATH      : string := "SIMPLE";
    PLLOUT_SELECT_PORTB: string := "GENCLK");
  port (
    PACKAGEPIN     : in  std_logic;
    PLLOUTGLOBALA  : out std_logic;
    PLLOUTCOREA    : out std_logic;
    PLLOUTGLOBALB  : out std_logic;
    PLLOUTCOREB    : out std_logic;
    RESETB         : in  std_logic;
    BYPASS         : in  std_logic;
    LOCK           : out std_logic);
end entity;

architecture behave of SB_PLL40_2_PAD is
  signal clk20 : std_logic := '0';
begin
  -- Port A: fixed reference passthrough (12 MHz CPU clock, unchanged).
  PLLOUTGLOBALA <= PACKAGEPIN;
  PLLOUTCOREA   <= PACKAGEPIN;

  -- Port B: free-running ~20 MHz (25 ns half period), independent of
  -- PACKAGEPIN -- same simplification as the old SB_PLL40_CORE sim model.
  gen: process
  begin
    clk20 <= '0';
    wait for 25 ns;
    clk20 <= '1';
    wait for 25 ns;
  end process;

  PLLOUTGLOBALB <= clk20;
  PLLOUTCOREB   <= clk20;

  lock_proc: process
  begin
    LOCK <= '0';
    wait for 5 us;
    LOCK <= '1';
    wait;
  end process;
end architecture;
