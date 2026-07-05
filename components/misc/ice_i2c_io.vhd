library ieee;
use ieee.std_logic_1164.all;

-- Open-drain I2C pads (SCL + SDA) for a bit-banged I2C master built from a
-- 2-bit tristate gpio2 device. The pad side is a 2-bit INOUT vector so socgen's
-- entity-pad mechanism wires it per-bit to pads named <signal>0/<signal>1 (the
-- same path as an sdram data bus). Bit 0 = SCL, bit 1 = SDA.
--
-- Each line is an iCE40 SB_IO driven open-drain: the gpio drives a constant-0
-- data value (d_o) and toggles the output-enable (d_t = tristate, 1 = release),
-- so OE=1 pulls low and OE=0 releases to the external pull-up (I2C idle-high).
-- The pad level is always read back on d_i so the master can sense ACKs / read
-- data / SDA. SB_IO is an unbound iCE40 primitive (sb_io_sim.vhd binds it for
-- GHDL), same pattern as ice_spi_io.
entity ice_i2c_io is
  port (
    d_o : in  std_logic_vector(1 downto 0);   -- gpio drive value (kept 0)
    d_t : in  std_logic_vector(1 downto 0);   -- gpio tristate (1 = release)
    d_i : out std_logic_vector(1 downto 0);   -- pad level read back to gpio
    pin : inout std_logic_vector(1 downto 0));-- 0 = SCL, 1 = SDA (open-drain pads)
end entity;

architecture rtl of ice_i2c_io is
  component SB_IO is
    generic (PIN_TYPE : std_logic_vector(5 downto 0) := "000000";
             PULLUP : std_logic := '0';
             IO_STANDARD : string := "SB_LVCMOS");
    port (PACKAGE_PIN : inout std_logic;
          OUTPUT_ENABLE : in std_logic;
          D_OUT_0 : in std_logic;
          D_IN_0 : out std_logic);
  end component;
  signal oe : std_logic_vector(1 downto 0);   -- output-enable = not tristate
begin
  oe <= not d_t;
  gen : for i in 0 to 1 generate
    io : SB_IO
      generic map (PIN_TYPE => "101001", PULLUP => '1')  -- output tristatable + input
      port map (PACKAGE_PIN => pin(i), OUTPUT_ENABLE => oe(i),
                D_OUT_0 => d_o(i), D_IN_0 => d_i(i));
  end generate;
end architecture;
