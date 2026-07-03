-- Self-checking testbench for eth_tx_phy: drives a small ROM frame through
-- the DUT, captures the differential mdi_p/mdi_n line at fixed offsets
-- (derived analytically from the DUT's cycle-accurate FSM timing), Manchester
-- decodes using the SAME convention as the RTL (second half-bit's mdi_p =
-- bit value, bytes assembled LSB-first), and checks against the ROM.

library ieee;
use ieee.std_logic_1164.all;
use ieee.numeric_std.all;

entity eth_tx_phy_tb is
end entity;

architecture sim of eth_tx_phy_tb is

  constant CLK_PERIOD : time := 50 ns;  -- 20 MHz
  constant NBYTES     : integer := 8;

  type rom_t is array (0 to NBYTES-1) of std_logic_vector(7 downto 0);
  -- Known frame: 0x55 preamble byte followed by a handful of data bytes.
  constant ROM : rom_t := (
    0 => x"55",
    1 => x"AB",
    2 => x"CD",
    3 => x"12",
    4 => x"34",
    5 => x"56",
    6 => x"78",
    7 => x"9A");

  signal clk_eth  : std_logic := '0';
  signal rst      : std_logic := '1';
  signal tx_start : std_logic := '0';
  signal tx_len   : unsigned(11 downto 0) := to_unsigned(NBYTES, 12);
  signal rd_addr  : unsigned(11 downto 0);
  signal rd_data  : std_logic_vector(7 downto 0) := (others => '0');
  signal busy     : std_logic;
  signal done     : std_logic;
  signal mdi_p    : std_logic;
  signal mdi_n    : std_logic;

  signal sim_done : boolean := false;

begin

  dut: entity work.eth_tx_phy
    port map (
      clk_eth  => clk_eth,
      rst      => rst,
      tx_start => tx_start,
      tx_len   => tx_len,
      rd_addr  => rd_addr,
      rd_data  => rd_data,
      busy     => busy,
      done     => done,
      mdi_p    => mdi_p,
      mdi_n    => mdi_n);

  -- 20 MHz clock
  clk_proc: process
  begin
    while not sim_done loop
      clk_eth <= '0';
      wait for CLK_PERIOD/2;
      clk_eth <= '1';
      wait for CLK_PERIOD/2;
    end loop;
    wait;
  end process;

  -- Byte-source ROM: a simple asynchronous (combinational) lookup buffer.
  -- rd_addr settles a full clk_eth period before the DUT's LOAD state
  -- samples rd_data (LOAD always runs exactly one clk_eth cycle after
  -- byte_idx/rd_addr changes), so rd_data is stable well in advance of
  -- LOAD's read -- satisfying the "rd_data valid the cycle after rd_addr"
  -- contract without needing extra registered latency in this stub.
  rom_proc: process (rd_addr)
  begin
    rd_data <= ROM(to_integer(rd_addr) mod NBYTES);
  end process;

  -- Stimulus + self-checking decode.
  stim: process
    variable t0      : time;
    variable si      : integer;
    variable got     : rom_t;
    variable bitval  : std_logic;
  begin
    -- reset
    rst <= '1';
    wait for 5*CLK_PERIOD;
    wait until rising_edge(clk_eth);
    rst <= '0';

    -- Idle a couple cycles, then pulse tx_start.
    wait until rising_edge(clk_eth);
    wait until rising_edge(clk_eth);
    tx_start <= '1';
    wait until rising_edge(clk_eth);
    -- This is the edge on which the DUT samples tx_start='1' (entering LOAD).
    t0 := now;
    tx_start <= '0';

    -- Manchester decode, per byte i, per bit j (LSB-first):
    --   S_i = 2 + 17*i   (first SEND half-bit slot of byte i; derived from
    --                      1 idle-transition slot + 1 LOAD slot before byte0,
    --                      and 16 bit-slots + 1 LOAD slot per subsequent byte)
    --   bit j of byte i's second half-bit slot = S_i + 2*j + 1
    --   decoded bit = mdi_p sampled mid-slot at that offset (second half's
    --   polarity equals the bit value, matching the RTL's
    --   diff <= b & (not b) assignment on the second half-bit).
    for i in 0 to NBYTES-1 loop
      si := 2 + 17*i;
      for j in 0 to 7 loop
        wait for (t0 + (real(si + 2*j + 1) + 0.5) * CLK_PERIOD) - now;
        bitval := mdi_p;
        assert mdi_p /= mdi_n
          report "eth_tx_phy_tb: illegal differential state (both/neither driven) "
                 & "at byte " & integer'image(i) & " bit " & integer'image(j)
          severity error;
        got(i)(j) := bitval;
      end loop;
      assert got(i) = ROM(i)
        report "eth_tx_phy_tb: byte mismatch at index " & integer'image(i)
        severity error;
    end loop;

    -- done should pulse once, busy should deassert afterwards.
    wait until done = '1';
    wait for CLK_PERIOD + 1 ns;
    assert done = '0'
      report "eth_tx_phy_tb: done did not deassert after one-cycle pulse"
      severity error;
    assert busy = '0'
      report "eth_tx_phy_tb: busy did not deassert after done"
      severity error;

    report "eth_tx_phy_tb PASSED" severity note;
    sim_done <= true;
    wait;
  end process;

end architecture;
