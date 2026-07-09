-- ice_spi_io: iCE40 SB_IO wrapper for reused config-SPI pins.
--
-- The iCESugar board multiplex the config-SPI pins (CS#/SCK/MOSI/MISO) that
-- load the FPGA bitstream with application XIP flash access. This wrapper
-- drives those physical pads via unbound SB_IO primitives, so at synthesis
-- the real iCE40 hard-block is used; at simulation (with --syn-binding), a
-- behavioral model stands in.
--
-- Port naming convention:
--   d_*   : digital logic signals (internal to SoC)
--   pin_* : physical pad signals (driven by SB_IO)

library ieee;
use ieee.std_logic_1164.all;

entity ice_spi_io is
  port (
    -- Internal logic signals (from spi_xip controller)
    d_cs_n  : in  std_logic;
    d_sck   : in  std_logic;
    d_mosi  : in  std_logic;
    d_miso  : out std_logic;

    -- Physical pad signals (to board pins via SB_IO)
    pin_cs_n  : inout std_logic;
    pin_sck   : inout std_logic;
    pin_mosi  : inout std_logic;
    pin_miso  : inout std_logic
  );
end entity ice_spi_io;

architecture rtl of ice_spi_io is

  -- Unbound SB_IO component: declared here, left unbound at synthesis,
  -- bound to a sim model at simulation via --syn-binding.
  component SB_IO is
    generic (
      PIN_TYPE : std_logic_vector(5 downto 0) := "000001");
    port (
      PACKAGE_PIN : inout std_logic;
      D_OUT_0     : in    std_logic;
      D_IN_0      : out   std_logic;
      OUTPUT_ENABLE : in  std_logic);
  end component;

begin

  -- CS# output pad (PIN_TYPE "011001" = unregistered output)
  u_cs_n : SB_IO
    generic map (PIN_TYPE => "011001")
    port map (
      PACKAGE_PIN   => pin_cs_n,
      D_OUT_0       => d_cs_n,
      D_IN_0        => open,
      OUTPUT_ENABLE => '1'
    );

  -- SCK output pad (PIN_TYPE "011001" = unregistered output)
  u_sck : SB_IO
    generic map (PIN_TYPE => "011001")
    port map (
      PACKAGE_PIN   => pin_sck,
      D_OUT_0       => d_sck,
      D_IN_0        => open,
      OUTPUT_ENABLE => '1'
    );

  -- MOSI output pad (PIN_TYPE "011001" = unregistered output)
  u_mosi : SB_IO
    generic map (PIN_TYPE => "011001")
    port map (
      PACKAGE_PIN   => pin_mosi,
      D_OUT_0       => d_mosi,
      D_IN_0        => open,
      OUTPUT_ENABLE => '1'
    );

  -- MISO input pad (PIN_TYPE "000001" = unregistered input)
  u_miso : SB_IO
    generic map (PIN_TYPE => "000001")
    port map (
      PACKAGE_PIN   => pin_miso,
      D_OUT_0       => '0',
      D_IN_0        => d_miso,
      OUTPUT_ENABLE => '0'
    );

end architecture rtl;
