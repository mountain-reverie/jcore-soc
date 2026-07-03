library ieee;
use ieee.std_logic_1164.all;
use ieee.numeric_std.all;
use std.textio.all;

entity eth_rx_phy_tb is
end entity;

architecture sim of eth_rx_phy_tb is

  constant CLK_PERIOD : time := 25 ns;   -- 40 MHz
  constant HALF_BIT    : time := 50 ns;  -- 100 ns bit period / 2

  signal clk_eth   : std_logic := '0';
  signal rst       : std_logic := '1';
  signal rx_in     : std_logic := '0';
  signal bit_o     : std_logic;
  signal bit_valid : std_logic;
  signal carrier   : std_logic;

  signal sim_done  : boolean := false;

  -- frame: 7x preamble (0x55) + SFD (0xD5) + 4 data bytes
  type byte_arr is array (natural range <>) of std_logic_vector(7 downto 0);
  constant FRAME : byte_arr := (
    x"55", x"55", x"55", x"55", x"55", x"55", x"55",
    x"D5",
    x"C0", x"1D", x"BE", x"EF");

begin

  dut: entity work.eth_rx_phy
    port map (
      clk_eth   => clk_eth,
      rst       => rst,
      rx_in     => rx_in,
      bit_o     => bit_o,
      bit_valid => bit_valid,
      carrier   => carrier);

  clk_eth <= not clk_eth after CLK_PERIOD / 2;

  -- ---------------------------------------------------------------------
  -- Manchester encoder driving rx_in -- MUST match eth_tx_phy.vhd SEND
  -- state convention exactly:
  --   first half-bit:  rx_in = not b
  --   second half-bit: rx_in = b
  -- bits are shifted LSB-first within each byte.
  -- ---------------------------------------------------------------------
  stim: process
    variable b       : std_logic;
    variable jitter  : time;
    variable bit_num : integer := 0;
  begin
    rst   <= '1';
    rx_in <= '0';
    wait for 200 ns;
    rst <= '0';
    wait for 100 ns;

    for byte_i in FRAME'range loop
      for bit_i in 0 to 7 loop
        b := FRAME(byte_i)(bit_i);   -- LSB-first

        -- add a few ns of jitter to a handful of edges to prove the DPLL
        -- tolerates real-world timing noise; keep most edges clean.
        jitter := 0 ns;
        if (bit_num mod 11) = 0 then
          jitter := 4 ns;
        elsif (bit_num mod 17) = 0 then
          jitter := -3 ns;
        end if;

        rx_in <= not b;
        wait for HALF_BIT + jitter;
        rx_in <= b;
        wait for HALF_BIT - jitter;

        bit_num := bit_num + 1;
      end loop;
    end loop;

    -- end of frame: return to idle (no more transitions) so carrier drops
    rx_in <= '0';
    wait for 2 us;

    sim_done <= true;
    wait;
  end process;

  -- ---------------------------------------------------------------------
  -- Capture recovered bits into a sliding window and look for the exact
  -- expected bit sequence (SFD + 4 data bytes, LSB-first per byte,
  -- concatenated) anywhere in the stream. Byte *framing* (deciding where
  -- byte boundaries fall) is Task 4's job -- this PHY only guarantees a
  -- correctly recovered, correctly ordered bit stream once locked, and
  -- lock may complete at an arbitrary point within the preamble. Matching
  -- a sliding window instead of assuming byte_count==FRAME index avoids
  -- baking a byte-alignment assumption into a bit-recovery test.
  -- ---------------------------------------------------------------------
  check: process
    constant EXP_BITS : integer := 5 * 8;  -- SFD + 4 data bytes
    variable expected  : std_logic_vector(EXP_BITS-1 downto 0);
    variable window    : std_logic_vector(EXP_BITS-1 downto 0) := (others => '0');
    variable carrier_seen_high : boolean := false;
    variable match_found       : boolean := false;
    variable total_bits        : integer := 0;
  begin
    -- build expected bit stream: bytes FRAME(7..11), LSB-first within
    -- each byte, earliest byte's bit0 at the MSB end of the window so a
    -- simple shift-left-in-from-LSB-of-window == oldest-bit-first compare
    -- works directly against the live shift register below.
    for byte_i in 7 to FRAME'high loop
      for bit_i in 0 to 7 loop
        expected((FRAME'high - byte_i) * 8 + (7 - bit_i)) := FRAME(byte_i)(bit_i);
      end loop;
    end loop;

    wait until rst = '0';

    while not sim_done loop
      wait until rising_edge(clk_eth);

      if carrier = '1' then
        carrier_seen_high := true;
      end if;

      if bit_valid = '1' then
        window := window(EXP_BITS-2 downto 0) & bit_o;
        total_bits := total_bits + 1;
        if total_bits >= EXP_BITS and window = expected then
          match_found := true;
        end if;
      end if;
    end loop;

    assert carrier_seen_high
      report "eth_rx_phy_tb: carrier never asserted" severity error;

    assert carrier = '0'
      report "eth_rx_phy_tb: carrier did not drop after end of frame" severity error;

    assert match_found
      report "eth_rx_phy_tb: FAILED (expected SFD+data bit sequence not found in recovered stream)"
      severity error;

    if carrier_seen_high and carrier = '0' and match_found then
      report "eth_rx_phy_tb PASSED" severity note;
    end if;

    wait;
  end process;

end architecture;
