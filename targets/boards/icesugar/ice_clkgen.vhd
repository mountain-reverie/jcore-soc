library ieee;
use ieee.std_logic_1164.all;
use ieee.numeric_std.all;

-- iCESugar clock/reset: the UP5K has a single PLL bel, which we use here (as
-- SB_PLL40_2_PAD) to share the 12 MHz oscillator pin (pin 35) between the CPU
-- clock and the Ethernet PHY clock, rather than instantiating a second PLL
-- inside eth_tx (which conflicted with this one for the same PLL bel).
--   clk_out (PLLOUTGLOBALA) = 12 MHz fixed reference passthrough -- CPU clock,
--     bit-identical in rate/phase relationship to the old plain passthrough.
--   clk_eth (PLLOUTGLOBALB) = ~40 MHz PLLOUT_SELECT_PORTB output -- Ethernet
--     PHY clock (eth_tx.clk_eth).
-- PLL params from `icepll -i 12 -o 40`:
--   DIVR=0 DIVF=52 DIVQ=4 FILTER_RANGE=1 (achieved 39.750 MHz, FEEDBACK SIMPLE)
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

  -- NOTE: the PLL below is deliberately NOT used for clk_out. See the
  -- "12 MHz passthrough" comment block further down before re-wiring it.
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

  -- Power-on reset counter. Counts UP from 0 and holds rst_out asserted
  -- until it saturates -- deliberately NOT a register preloaded with ones.
  -- iCE40 flip-flops power up to 0 and have no init-to-1 capability, so the
  -- previous 4-bit shift register initialised to "1111" could come out of
  -- configuration already drained, leaving the SoC with NO reset pulse at
  -- all. The CPU survives that (it just starts fetching) and gpio2 survives
  -- it (its ack is unconditional), but uartlite does not: it accepts a bus
  -- cycle only on a rising edge of en, and with its state never reset that
  -- edge never arrives, so the first store to the UART hangs the CPU forever.
  signal por_cnt  : unsigned(7 downto 0) := (others => '0');

begin

  -- 12 MHz passthrough (see the entity header): clk_out MUST be the 12 MHz
  -- oscillator reference, not a PLL output.
  --
  -- This previously instantiated SB_PLL40_2_PAD and took clk_out from
  -- PLLOUTGLOBALA, described as a "12 MHz fixed reference passthrough". It is
  -- not one. Both PLL output ports come off the same VCO, so with
  -- DIVR=0/DIVF=52/DIVQ=4 port A emitted 12*53/16 = 39.75 MHz -- and since the
  -- component declaration omitted PLLOUT_SELECT_PORTA, it defaulted to GENCLK
  -- rather than anything passthrough-like. The J1 SoC closes timing at about
  -- 12.7 MHz, so clocking it at 39.75 MHz left it completely dead on hardware
  -- while every constraint still "passed" (nextpnr was told 12 MHz).
  --
  -- Ethernet: clk_eth is unused by this board's design.yaml (pad_ring maps it
  -- to `open`). An Ethernet variant needs a real PLL for clk_eth AND a 12 MHz
  -- clk_out, which one SB_PLL40_2_PAD cannot provide -- the reference is
  -- consumed by the PAD primitive. Feed SB_PLL40_CORE from a regular input
  -- instead, so the raw reference stays available for clk_out.
  clk_out <= clk_in;
  clk_eth <= clk_in;

  process (clk_in)
  begin
    if rising_edge(clk_in) then
      if por_cnt /= x"ff" then
        por_cnt <= por_cnt + 1;
      end if;
    end if;
  end process;

  -- No PLL, so no LOCK to gate on: release once the counter saturates.
  rst_out <= '0' when por_cnt = x"ff" else '1';

end architecture;
