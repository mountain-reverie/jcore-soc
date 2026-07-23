-------------------------------------------------------------------------------
-- gf180_qspi_io.vhd
--
-- Behavioral (NOT synthesizable-required) GF180 quad-SPI bidirectional pad
-- wrapper for the QSPI flash controller. Maps the controller's split triplet
-- (fl_io_o / fl_io_oe / fl_io_i) to a true VHDL inout pad (pad_io).
--
-- This wrapper is the RTL-visible, technology-independent interface for
-- the flash IO lines. In the actual SoC, the GF180 pad ring (IO cells)
-- binds the inout pad signals to real GF180 IO_CTRL primitives with
-- pull-ups, slew-rate control, and drive strength.
--
-- Controller side (fl_*): split triplet convention
--   fl_cs_n  : in  std_logic              (chip select, low-active)
--   fl_sck   : in  std_logic              (SPI clock)
--   fl_io_o  : in  std_logic_vector(3:0) (data the controller drives)
--   fl_io_oe : in  std_logic_vector(3:0) (per-line output-enable)
--   fl_io_i  : out std_logic_vector(3:0) (data sampled by the controller)
--
-- Pad side (pad_*): resolved inout convention
--   pad_cs_n  : out std_logic              (passthrough from fl_cs_n)
--   pad_sck   : out std_logic              (passthrough from fl_sck)
--   pad_io    : inout std_logic_vector(3:0) (bidirectional data bus)
--
-- Behavioral logic:
--   pad_cs_n <= fl_cs_n;
--   pad_sck  <= fl_sck;
--   For each IO line k (0..3):
--     pad_io(k) <= fl_io_o(k) when fl_io_oe(k) = '1' else 'Z';
--     fl_io_i(k) <= pad_io(k);
--
-- The 'Z' (high-impedance) driver enables real VHDL inout resolution:
-- when the controller drives (fl_io_oe='1'), the pad reflects the driven
-- value; when tristated (fl_io_oe='0'), the pad may be driven by an
-- external device (e.g., the flash model), and the controller samples
-- the resolved value on fl_io_i.
-------------------------------------------------------------------------------

library ieee;
use ieee.std_logic_1164.all;

entity gf180_qspi_io is
  port (
    -- Controller side (split triplet)
    fl_cs_n  : in  std_logic;
    fl_sck   : in  std_logic;
    fl_io_o  : in  std_logic_vector(3 downto 0);
    fl_io_oe : in  std_logic_vector(3 downto 0);
    fl_io_i  : out std_logic_vector(3 downto 0);

    -- Pad side (resolved inout)
    pad_cs_n : out std_logic;
    pad_sck  : out std_logic;
    pad_io   : inout std_logic_vector(3 downto 0));
end entity;

architecture behavioral of gf180_qspi_io is
begin

  -- Passthrough chip-select and clock
  pad_cs_n <= fl_cs_n;
  pad_sck  <= fl_sck;

  -- Bidirectional IO lines: tristate on controller, with real inout resolution
  gen_io : for i in 0 to 3 generate
    -- Drive pad_io from controller when fl_io_oe(i) is '1', else tristated
    pad_io(i) <= fl_io_o(i) when fl_io_oe(i) = '1' else 'Z';
    -- Sample the resolved pad back to the controller
    fl_io_i(i) <= pad_io(i);
  end generate;

end architecture;
