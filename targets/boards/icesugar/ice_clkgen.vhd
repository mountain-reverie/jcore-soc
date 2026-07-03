library ieee;
use ieee.std_logic_1164.all;

-- iCESugar clock/reset: the UP5K has a single PLL bel, which we use here (as
-- SB_PLL40_2_PAD) to share the 12 MHz oscillator pin (pin 35) between the CPU
-- clock and the Ethernet PHY clock, rather than instantiating a second PLL
-- inside eth_tx (which conflicted with this one for the same PLL bel).
--   clk_out (PLLOUTGLOBALA) = 12 MHz fixed reference passthrough -- CPU clock,
--     bit-identical in rate/phase relationship to the old plain passthrough.
--   clk_eth (PLLOUTGLOBALB) = ~20 MHz PLLOUT_SELECT_PORTB output -- Ethernet
--     PHY clock (eth_tx.clk_eth).
-- PLL params from `icepll -i 12 -o 20`:
--   DIVR=0 DIVF=52 DIVQ=5 FILTER_RANGE=1 (achieved 19.875 MHz, FEEDBACK SIMPLE)
-- rst_out is a power-on reset held for a few cycles then released, gated by
-- the PLL LOCK signal, synchronized to clk_out.
entity ice_clkgen is
  port (
    clk_in  : in  std_logic;
    clk_out : out std_logic;
    clk_eth : out std_logic;
    rst_out : out std_logic);
end entity;

architecture rtl of ice_clkgen is

  component SB_PLL40_2_PAD is
    generic (
      DIVR                : std_logic_vector(3 downto 0) := "0000";
      DIVF                : std_logic_vector(6 downto 0) := "0000000";
      DIVQ                : std_logic_vector(2 downto 0) := "000";
      FILTER_RANGE        : std_logic_vector(2 downto 0) := "000";
      FEEDBACK_PATH       : string := "SIMPLE";
      PLLOUT_SELECT_PORTB : string := "GENCLK");
    port (
      PACKAGEPIN    : in  std_logic;
      PLLOUTGLOBALA : out std_logic;
      PLLOUTCOREA   : out std_logic;
      PLLOUTGLOBALB : out std_logic;
      PLLOUTCOREB   : out std_logic;
      RESETB        : in  std_logic;
      BYPASS        : in  std_logic;
      LOCK          : out std_logic);
  end component;

  signal clk_cpu  : std_logic;
  signal pll_lock : std_logic;
  signal por      : std_logic_vector(3 downto 0) := (others => '1');

begin

  pll: SB_PLL40_2_PAD
    generic map (
      FEEDBACK_PATH       => "SIMPLE",
      PLLOUT_SELECT_PORTB => "GENCLK",
      DIVR                => "0000",     -- 0
      DIVF                => "0110100",  -- 52
      DIVQ                => "101",      -- 5
      FILTER_RANGE        => "001")      -- 1
    port map (
      PACKAGEPIN    => clk_in,
      PLLOUTGLOBALA => clk_cpu,
      PLLOUTCOREA   => open,
      PLLOUTGLOBALB => clk_eth,
      PLLOUTCOREB   => open,
      RESETB        => '1',
      BYPASS        => '0',
      LOCK          => pll_lock);

  clk_out <= clk_cpu;

  process (clk_cpu)
  begin
    if rising_edge(clk_cpu) then
      por <= por(2 downto 0) & '0';
    end if;
  end process;
  -- Reset is released only once the POR shift has drained AND the PLL has
  -- locked: rst asserted = POR-active OR not-locked (i.e. POR AND LOCK gates
  -- the release). Cheaper than resetting the shift register on !lock.
  rst_out <= por(3) or (not pll_lock);

end architecture;
