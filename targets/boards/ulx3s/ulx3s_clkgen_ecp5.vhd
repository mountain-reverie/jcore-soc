-- ECP5 architecture of clkgen: EHXPLLL. 25 MHz reference, ~25 MHz output
-- (CLKOP) for first boot (CLKI_DIV/CLKFB_DIV/CLKOP_DIV = 1:1:24, 600 MHz VCO).
-- Analyzed only by the synth flow; the sim flow analyzes the sim arch instead,
-- so default binding selects the right one without a configuration (ghdl cannot
-- elaborate EHXPLLL).
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
      CLKI_DIV => 1, CLKFB_DIV => 1, CLKOP_DIV => 24,
      CLKOP_ENABLE => "ENABLED", CLKOP_CPHASE => 0, CLKOP_FPHASE => 0,
      FEEDBK_PATH => "CLKOP")
    port map (
      CLKI => clk_in, CLKFB => clkop_i, RST => rst_in, ENCLKOP => '1',
      CLKOP => clkop_i, LOCK => locked);
  clk <= clkop_i;
end architecture;
