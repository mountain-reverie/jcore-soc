library ieee;
use ieee.std_logic_1164.all;

entity clkgen is
  port (
    clk_in : in  std_logic;   -- 25 MHz board oscillator
    rst_in : in  std_logic;   -- async external reset (active high)
    clk    : out std_logic;   -- CPU clock
    locked : out std_logic);  -- high when clk is stable
end entity;

-- Simulation: passthrough, lock after a couple cycles. ghdl cannot
-- elaborate EHXPLLL, so testbenches bind this architecture.
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

-- ECP5: EHXPLLL. 25 MHz reference, ~25 MHz output (CLKOP) for first boot.
-- (CLKI_DIV/CLKFB_DIV/CLKOP_DIV are conservative 1:1:24 with a 600 MHz VCO.)
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
