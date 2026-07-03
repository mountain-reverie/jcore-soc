-- Self-checking testbench for eth_tx (the CPU-bus TX device wrapping the
-- Manchester PHY, an inferred dual-clock buffer, and the SB_PLL40_CORE model).
--
-- Drives db_i at 12 MHz: reset the write pointer, write a known frame as 32-bit
-- big-endian words to TX_DATA, set TX_LEN, pulse TX_GO. Captures mdi_p/mdi_n and
-- Manchester-decodes it reusing the eth_tx_phy_tb convention (second half-bit's
-- mdi_p = bit value, bytes LSB-first). The DUT's PLL model free-runs a
-- deterministic 20 MHz clk_eth (edges at 25 ns + k*50 ns) so the PHY slot timing
-- is reconstructed from the first non-idle line transition after TX_GO.
--
-- Elaborate/run with --syn-binding so the SB_PLL40_CORE component binds to the
-- behavioural model in sb_pll40_core_sim.vhd.
library ieee;
use ieee.std_logic_1164.all;
use ieee.numeric_std.all;
use work.cpu2j0_pack.all;

entity eth_tx_tb is
end entity;

architecture sim of eth_tx_tb is

  constant CLK12 : time := 83333 ps;   -- ~12 MHz CPU bus clock
  constant PETH  : time := 50 ns;      -- 20 MHz clk_eth period (PLL model)
  -- Not a multiple of 4: exercises partial-word / byte-lane-boundary
  -- handling in the 4-lane RAM read (frame length 6 stops mid-word-1, i.e.
  -- only lanes 0/1 of the second word are ever read).
  constant NBYTES : integer := 6;

  type rom_t is array (0 to NBYTES-1) of std_logic_vector(7 downto 0);
  constant ROM : rom_t := (
    0 => x"55", 1 => x"AB", 2 => x"CD", 3 => x"12",
    4 => x"34", 5 => x"56");

  -- Frame as big-endian 32-bit words (byte0 = d(31:24)).
  constant WORD0 : std_logic_vector(31 downto 0) := x"55ABCD12";
  constant WORD1 : std_logic_vector(31 downto 0) := x"3456789A";

  signal clk     : std_logic := '0';
  signal rst     : std_logic := '1';
  signal db_i    : cpu_data_o_t := NULL_DATA_O;
  signal db_o    : cpu_data_i_t;
  signal mdi_p   : std_logic;
  signal mdi_n   : std_logic;
  signal tx_done : std_logic;

  signal sim_done : boolean := false;

  -- first non-idle line time (start of first SEND slot)
  signal t_first : time := 0 ns;
  signal detect_en : boolean := false;
  signal got_first : boolean := false;

  signal tx_done_seen : boolean := false;

begin

  dut: entity work.eth_tx
    port map (
      clk => clk, rst => rst, db_i => db_i, db_o => db_o,
      mdi_p => mdi_p, mdi_n => mdi_n, tx_done => tx_done);

  clk_proc: process
  begin
    while not sim_done loop
      clk <= '0'; wait for CLK12/2;
      clk <= '1'; wait for CLK12/2;
    end loop;
    wait;
  end process;

  -- Capture the first non-idle transition of the line after TX_GO.
  detect_proc: process (mdi_p, mdi_n)
  begin
    if detect_en and not got_first then
      if mdi_p = '1' or mdi_n = '1' then
        t_first   <= now;
        got_first <= true;
      end if;
    end if;
  end process;

  -- Latch a tx_done pulse.
  done_proc: process (clk)
  begin
    if rising_edge(clk) then
      if tx_done = '1' then
        tx_done_seen <= true;
      end if;
    end if;
  end process;

  stim: process
    -- 32-bit write: db_o.ack is combinational (db_o.ack <= db_i.en, per the
    -- uart.vhd/pio.vhd/gpio2.vhd convention), so poll/wait for it instead of
    -- assuming a fixed cycle count -- this actually exercises ack timing.
    procedure bus_write(addr : std_logic_vector(11 downto 0);
                        data : std_logic_vector(31 downto 0)) is
    begin
      wait until rising_edge(clk);
      db_i.en <= '1'; db_i.wr <= '1'; db_i.rd <= '0';
      db_i.we <= "1111"; db_i.a <= x"00000" & addr; db_i.d <= data;
      wait until db_o.ack = '1';       -- combinational ack acknowledges the request
      wait until rising_edge(clk);
      db_i.en <= '0'; db_i.wr <= '0'; db_i.we <= "0000";
    end procedure;

    procedure bus_read(addr : std_logic_vector(11 downto 0);
                       result : out std_logic_vector(31 downto 0)) is
    begin
      wait until rising_edge(clk);
      db_i.en <= '1'; db_i.rd <= '1'; db_i.wr <= '0';
      db_i.a  <= x"00000" & addr;
      wait until db_o.ack = '1';       -- combinational ack acknowledges the request
      wait until rising_edge(clk);     -- rdata_r registers the read data on this edge
      db_i.en <= '0'; db_i.rd <= '0';
      wait until rising_edge(clk);
      result := db_o.d;
    end procedure;

    variable rdw   : std_logic_vector(31 downto 0);
    variable got   : rom_t;
    variable poll  : integer;
  begin
    rst <= '1';
    for i in 0 to 8 loop wait until rising_edge(clk); end loop;
    rst <= '0';
    -- wait for PLL lock window
    wait for 7 us;

    -- program the frame
    bus_write(x"804", x"00000001");   -- reset write pointer
    bus_write(x"800", WORD0);         -- TX_DATA word 0
    bus_write(x"800", WORD1);         -- TX_DATA word 1
    bus_write(x"808", x"00000006");   -- TX_LEN = 6 bytes (not a multiple of 4)

    -- enable line-detection just before GO, then fire GO
    detect_en <= true;
    bus_write(x"80C", x"00000001");   -- TX_GO

    -- wait until the first data half-bit is captured
    wait until got_first;
    -- t_first is the start of the first SEND slot (phy_tb si=4 slot, i.e.
    -- byte0/bit0's first half-bit -- the gapless redesign runs exactly 16
    -- half-bit slots per byte, no LOAD gap between bytes).
    -- sample(i,j) = t_first + (16*i + 2*j + 1.5)*PETH ; bit = mdi_p (LSB-first)
    for i in 0 to NBYTES-1 loop
      for j in 0 to 7 loop
        wait for (t_first + (real(16*i + 2*j) + 1.5) * PETH) - now;
        assert mdi_p /= mdi_n
          report "eth_tx_tb: illegal differential state at byte "
                 & integer'image(i) & " bit " & integer'image(j)
          severity failure;
        got(i)(j) := mdi_p;
      end loop;
      assert got(i) = ROM(i)
        report "eth_tx_tb: byte mismatch at index " & integer'image(i)
               & " got " & integer'image(to_integer(unsigned(got(i))))
               & " exp " & integer'image(to_integer(unsigned(ROM(i))))
        severity failure;
    end loop;

    -- poll busy (0x810) until clear
    poll := 0;
    loop
      bus_read(x"810", rdw);
      exit when rdw(0) = '0';
      poll := poll + 1;
      assert poll < 100000 report "eth_tx_tb: busy stuck high" severity failure;
    end loop;

    -- tx_done should have pulsed by now (busy falling edge)
    wait for 2 * CLK12;
    assert tx_done_seen
      report "eth_tx_tb: tx_done never pulsed" severity failure;

    report "eth_tx_tb PASSED" severity note;
    sim_done <= true;
    wait;
  end process;

end architecture;
