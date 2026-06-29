-- Sim-only behavioural model of the Lattice ECP5 EHXPLLL primitive.
--
-- The generated pad_ring binds clkgen(ecp5), whose ECP5 architecture
-- instantiates the EHXPLLL hard PLL. The synthesis flow (yosys/nextpnr) supplies
-- the real primitive; ghdl cannot elaborate it, so the simulation flow analyzes
-- this stand-in instead (deliberately excluded from the synth filelist, like
-- components/sdram/sdram_model.vhd).
--
-- The ULX3S testbenches feed clk_25mhz at the POST-PLL rate (20 MHz), i.e. they
-- bypass the PLL ratio, so this model simply passes CLKI straight through to
-- CLKOP and asserts LOCK after a short delay. The CLKx_DIV generics are accepted
-- and ignored (the tb already provides the divided clock).
library ieee;
use ieee.std_logic_1164.all;

entity EHXPLLL is
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
end entity;

architecture behave of EHXPLLL is
begin
  CLKOP <= CLKI;
  -- async-deasserted lock: low while RST, then high a few ns after release.
  LOCK <= '0' when RST = '1' else '1' after 100 ns;
end architecture;
