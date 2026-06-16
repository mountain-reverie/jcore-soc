-- ECP5 architecture of clkgen: EHXPLLL. 25 MHz reference -> 20 MHz output
-- (CLKOP): CLKI_DIV/CLKFB_DIV/CLKOP_DIV = 5:4:30 -> PFD 5 MHz, VCO 600 MHz,
-- CLKOP 20 MHz. 20 MHz is the M2 target: the full SoC (CPU+caches+SDRAM+AIC+
-- GPIO) closes timing at ~22.4 MHz on the -6 85F (the L1 cache single-clock CDC
-- half-cycle path limits it; a dedicated CDC-simplification follow-up reclaims
-- 25+ MHz). Analyzed only by the synth flow; the sim flow analyzes the sim arch
-- instead, so default binding selects the right one (ghdl cannot elaborate
-- EHXPLLL).
library ieee;
use ieee.std_logic_1164.all;

architecture ecp5 of clkgen is
  component EHXPLLL
    generic (
      CLKI_DIV : integer := 1; CLKFB_DIV : integer := 1; CLKOP_DIV : integer := 1;
      CLKOP_ENABLE : string := "ENABLED"; CLKOP_CPHASE : integer := 0;
      CLKOP_FPHASE : integer := 0; FEEDBK_PATH : string := "CLKOP");
    port (
      CLKI : in std_logic; CLKFB : in std_logic;
      PHASESEL1 : in std_logic := '0'; PHASESEL0 : in std_logic := '0';
      PHASEDIR : in std_logic := '0'; PHASESTEP : in std_logic := '0';
      PHASELOADREG : in std_logic := '0'; STDBY : in std_logic := '0';
      PLLWAKESYNC : in std_logic := '0'; RST : in std_logic := '0';
      ENCLKOP : in std_logic := '0';
      CLKOP : out std_logic; LOCK : out std_logic);
  end component;
  signal clkop_i : std_logic;
begin
  pll : EHXPLLL
    generic map (
      CLKI_DIV => 5, CLKFB_DIV => 4, CLKOP_DIV => 30,
      CLKOP_ENABLE => "ENABLED", CLKOP_CPHASE => 0, CLKOP_FPHASE => 0,
      FEEDBK_PATH => "CLKOP")
    port map (
      CLKI => clk_in, CLKFB => clkop_i, RST => rst_in, ENCLKOP => '1',
      CLKOP => clkop_i, LOCK => locked);
  clk <= clkop_i;
end architecture;
