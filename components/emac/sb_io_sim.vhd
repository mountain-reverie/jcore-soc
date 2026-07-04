library ieee;
use ieee.std_logic_1164.all;

-- Behavioral GHDL sim model of the iCE40 SB_IO primitive, sufficient for the
-- Ethernet RX LVDS input (ice_lvds_in.vhd): a simple unregistered input that
-- passes the pad through to D_IN_0. Bound via --syn-binding for simulation;
-- EXCLUDED from synth filelists (yosys maps the real SB_IO), like
-- sb_pll40_2_pad_sim.vhd. Only the ports/generics used by ice_lvds_in are
-- modelled meaningfully; the rest are accepted and ignored.
entity SB_IO is
  generic (
    PIN_TYPE    : std_logic_vector(5 downto 0) := "000000";
    PULLUP      : std_logic := '0';
    NEG_TRIGGER : std_logic := '0';
    IO_STANDARD : string    := "SB_LVCMOS");
  port (
    PACKAGE_PIN       : in  std_logic;
    LATCH_INPUT_VALUE : in  std_logic := '0';
    CLOCK_ENABLE      : in  std_logic := '0';
    INPUT_CLK         : in  std_logic := '0';
    OUTPUT_CLK        : in  std_logic := '0';
    OUTPUT_ENABLE     : in  std_logic := '0';
    D_OUT_0           : in  std_logic := '0';
    D_OUT_1           : in  std_logic := '0';
    D_IN_0            : out std_logic;
    D_IN_1            : out std_logic);
end entity;

architecture sim of SB_IO is
begin
  D_IN_0 <= PACKAGE_PIN;   -- unregistered input passthrough
  D_IN_1 <= PACKAGE_PIN;
end architecture;
