library ieee;
use ieee.std_logic_1164.all;

-- Behavioral GHDL sim model of the iCE40 SB_IO primitive. Originally just an
-- unregistered input passthrough for the Ethernet RX LVDS input
-- (ice_lvds_in.vhd, which ties OUTPUT_ENABLE => '0' and never drives
-- PACKAGE_PIN). Extended to a real tristate/open-drain model so
-- ice_i2c_io.vhd (bit-banged DS3231 I2C: PIN_TYPE "101001", tristatable
-- output + input, PULLUP => '1') works in sim too: when OUTPUT_ENABLE='1'
-- the pad is driven with D_OUT_0; when OUTPUT_ENABLE='0' it is released
-- ('Z'), or weakly pulled to 'H' if PULLUP='1' (matching the iCE40's
-- internal pad pull-up used for open-drain I2C). D_IN_0/D_IN_1 always pass
-- the resolved pad level through unregistered.
--
-- Bound via --syn-binding for simulation; EXCLUDED from synth filelists
-- (yosys maps the real SB_IO), like sb_pll40_2_pad_sim.vhd. Clocked output
-- registers (OUTPUT_CLK), latches and NEG_TRIGGER are not modelled -- no
-- current board use needs them.
entity SB_IO is
  generic (
    PIN_TYPE    : std_logic_vector(5 downto 0) := "000000";
    PULLUP      : std_logic := '0';
    NEG_TRIGGER : std_logic := '0';
    IO_STANDARD : string    := "SB_LVCMOS");
  port (
    PACKAGE_PIN       : inout std_logic;
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
  PACKAGE_PIN <= D_OUT_0 when OUTPUT_ENABLE = '1' else
                 'H'      when PULLUP = '1' else
                 'Z';
  D_IN_0 <= PACKAGE_PIN;   -- unregistered input passthrough
  D_IN_1 <= PACKAGE_PIN;
end architecture;
