-- mem_region_mux: purely combinational region decode/mux sitting between
-- ddr_ram_mux's ddr_bus (req/bst upstream, resp/ack_r downstream) and two
-- mem-bus targets: sdram_ctrl (default) and qspi_flash_ctrl (flash region,
-- decoded off req.a via FLASH_BASE/FLASH_MASK generics). See
-- components/misc/qspi_flash_ctrl.vhd (Task 1) for the burst mem-bus
-- contract this mux fans req/bst out to and merges resp/ack_r back from
-- (req:cpu_data_o_t, bst:std_logic, resp:cpu_data_i_t, ack_r:std_logic --
-- identical to sdram_ctrl's contract).
--
-- Region decode: (req.a and FLASH_MASK) = (FLASH_BASE and FLASH_MASK)
-- selects flash; everything else (including idle/no request) defaults to
-- sdram. Only the addressed target sees req.en asserted; the other target's
-- request is gated off (en de-asserted, rest of req held at NULL_DATA_O) so
-- it stays idle. resp/ack_r are muxed back from whichever target is
-- currently addressed. No clocked state here -- sdram_ctrl and
-- qspi_flash_ctrl each own their own handshake FSMs.
library ieee;
use ieee.std_logic_1164.all;
use work.cpu2j0_pack.all;
use work.data_bus_pack.all;

entity mem_region_mux is
  generic (
    FLASH_BASE : std_logic_vector(31 downto 0) := (others => '0');
    FLASH_MASK : std_logic_vector(31 downto 0) := (others => '0'));  -- '1' bits are compared
  port (
    -- upstream (from ddr_ram_mux)
    req    : in  cpu_data_o_t;
    bst    : in  std_logic;
    resp   : out cpu_data_i_t;
    ack_r  : out std_logic;

    -- downstream: sdram_ctrl
    sdram_req   : out cpu_data_o_t;
    sdram_bst   : out std_logic;
    sdram_resp  : in  cpu_data_i_t;
    sdram_ack_r : in  std_logic;

    -- downstream: qspi_flash_ctrl
    flash_req   : out cpu_data_o_t;
    flash_bst   : out std_logic;
    flash_resp  : in  cpu_data_i_t;
    flash_ack_r : in  std_logic);
end entity;

architecture rtl of mem_region_mux is
  signal is_flash : std_logic;
begin

  is_flash <= '1' when (req.a and FLASH_MASK) = (FLASH_BASE and FLASH_MASK) else '0';

  -- request fan-out: gate the un-addressed target's request off so it stays
  -- idle; the addressed target sees the full request untouched.
  sdram_req <= req when is_flash = '0' else
               (en => '0', a => req.a, rd => '0', wr => '0', we => "0000", d => req.d);
  flash_req <= req when is_flash = '1' else
               (en => '0', a => req.a, rd => '0', wr => '0', we => "0000", d => req.d);

  sdram_bst <= bst when is_flash = '0' else '0';
  flash_bst <= bst when is_flash = '1' else '0';

  -- resp/ack_r merge: pass through whichever region is currently addressed.
  resp  <= flash_resp when is_flash = '1' else sdram_resp;
  ack_r <= flash_ack_r when is_flash = '1' else sdram_ack_r;

end architecture;
