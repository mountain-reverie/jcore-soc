library ieee;
use ieee.std_logic_1164.all;

-- iCE40 LVDS differential input for the Ethernet RX pair. MDI1_P is on the
-- positive pad (pin 42); on iCE40 the negative leg (pin 38) is the AUTOMATIC
-- differential complement of the positive pin -- nothing is instantiated on it,
-- but it must be left reserved. This wraps an SB_IO primitive with
-- IO_STANDARD => "SB_LVDS_INPUT" so the synthesized bitstream slices the true
-- differential signal (rather than a single-ended sample of one leg, which is
-- what a plain `mdi1 <= pin_mdi1_p` wire gives).
--
-- SB_IO is an unbound iCE40 hard primitive at synth (mapped by yosys), exactly
-- like SB_PLL40_2_PAD in ice_clkgen. For GHDL simulation a behavioral model
-- (components/emac/sb_io_sim.vhd) is bound via --syn-binding; it just passes the
-- pad through to d_out. PACKAGE_PIN is the iCE40 pad's bidirectional port, but we
-- only ever read it, so it is declared `in` here (input-only use).
entity ice_lvds_in is
  port (
    pad_p : in  std_logic;   -- LVDS positive pad (pin 42); pin 38 = auto complement
    d_out : out std_logic);  -- recovered digital level (feeds eth_rx rx_in)
end entity;

architecture rtl of ice_lvds_in is
  component SB_IO is
    generic (
      PIN_TYPE    : std_logic_vector(5 downto 0) := "000000";
      PULLUP      : std_logic := '0';
      NEG_TRIGGER : std_logic := '0';
      IO_STANDARD : string    := "SB_LVCMOS");
    port (
      PACKAGE_PIN       : in  std_logic;
      LATCH_INPUT_VALUE : in  std_logic;
      CLOCK_ENABLE      : in  std_logic;
      INPUT_CLK         : in  std_logic;
      OUTPUT_CLK        : in  std_logic;
      OUTPUT_ENABLE     : in  std_logic;
      D_OUT_0           : in  std_logic;
      D_OUT_1           : in  std_logic;
      D_IN_0            : out std_logic;
      D_IN_1            : out std_logic);
  end component;
begin
  io : SB_IO
    generic map (
      PIN_TYPE    => "000001",          -- [5:2]=0000 no output, [1:0]=01 unregistered input
      IO_STANDARD => "SB_LVDS_INPUT")
    port map (
      PACKAGE_PIN       => pad_p,
      LATCH_INPUT_VALUE => '0',
      CLOCK_ENABLE      => '0',
      INPUT_CLK         => '0',
      OUTPUT_CLK        => '0',
      OUTPUT_ENABLE     => '0',
      D_OUT_0           => '0',
      D_OUT_1           => '0',
      D_IN_0            => d_out,
      D_IN_1            => open);
end architecture;
