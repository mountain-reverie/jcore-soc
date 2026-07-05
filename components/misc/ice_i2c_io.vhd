library ieee;
use ieee.std_logic_1164.all;

-- Open-drain I2C pads (SCL + SDA) for a bit-banged I2C master built from a
-- 2-bit gpio2 device. Each line is an iCE40 SB_IO driven as open-drain: the
-- gpio drives a constant-0 data value and toggles the output-enable, so
-- OE=1 pulls the line low and OE=0 releases it to the external pull-up (the
-- I2C idle-high level); the pad level is always read back on d_i so the master
-- can sense ACKs / clock-stretch / SDA.
--
-- gpio2 convention: d_t is the TRISTATE control (1 = high-Z / released), so the
-- SB_IO OUTPUT_ENABLE = not d_t. d_o carries the drive value (the I2C driver
-- keeps it 0 and works the line via d_t). SB_IO is an unbound iCE40 primitive
-- (sb_io_sim.vhd binds it for GHDL), the same pattern as ice_spi_io.
-- Bit 0 = SCL, bit 1 = SDA.
entity ice_i2c_io is
  port (
    d_o : in  std_logic_vector(1 downto 0);   -- gpio drive value (kept 0)
    d_t : in  std_logic_vector(1 downto 0);   -- gpio tristate (1 = release)
    d_i : out std_logic_vector(1 downto 0);   -- pad level read back to gpio
    pin_scl : inout std_logic;
    pin_sda : inout std_logic);
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
  scl : SB_IO
    generic map (PIN_TYPE => "101001", PULLUP => '1')  -- output tristatable + input
    port map (PACKAGE_PIN => pin_scl, OUTPUT_ENABLE => oe(0),
              D_OUT_0 => d_o(0), D_IN_0 => d_i(0));
  sda : SB_IO
    generic map (PIN_TYPE => "101001", PULLUP => '1')
    port map (PACKAGE_PIN => pin_sda, OUTPUT_ENABLE => oe(1),
              D_OUT_0 => d_o(1), D_IN_0 => d_i(1));
end architecture;
