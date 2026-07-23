-------------------------------------------------------------------------------
-- qspi_flash_ctrl_tb.vhd
--
-- GATE testbench for Task 3 (components/misc/qspi_flash_ctrl.vhd):
-- instantiates qspi_flash_ctrl (LANES=4) against the Task-1
-- qspi_flash_model and drives it purely over the jcore bus (db_i/db_o),
-- proving the double ping-pong line buffer + sequential prefetch +
-- multi-cycle (deferred) ack.
--
-- Expected data: qspi_flash_model's deterministic pattern is
--   byte(addr) = addr(7 downto 0) xor addr(15 downto 8)
-- db_o.d ENDIANNESS (documented in qspi_flash_ctrl.vhd): big-endian --
-- for a word at flash byte address A, db_o.d(31 downto 24) = byte(A),
-- db_o.d(23 downto 16) = byte(A+1), db_o.d(15 downto 8) = byte(A+2),
-- db_o.d(7 downto 0) = byte(A+3).
--
-- Three access patterns, all against FLASH_BASE = 0:
--   (a) SEQUENTIAL burst of 20 words starting mid-line (word index 4 of
--       line 0, byte offset 16) so it crosses two 32-byte line
--       boundaries -- proves ping-pong swap + prefetch.
--   (b) RANDOM reads to 4 scattered addresses in different, non-adjacent
--       lines -- proves miss/flush/restart.
--   (c) Re-reads of words within an already-buffered line -- proves NO
--       new flash transaction occurs on a buffer hit. A CS
--       falling-edge counter is kept in this tb (qspi_flash_model is
--       left unmodified) and asserted not to increase across the
--       repeated reads.
-------------------------------------------------------------------------------

library ieee;
use ieee.std_logic_1164.all;
use ieee.numeric_std.all;
use work.cpu2j0_pack.all;

entity qspi_flash_ctrl_tb is
end entity;

architecture tb of qspi_flash_ctrl_tb is

  constant CLK_PERIOD : time := 20 ns;
  constant FLASH_BASE : natural := 0;

  signal clk : std_logic := '0';
  signal rst : std_logic := '1';

  signal db_i : cpu_data_o_t := null_data_o;
  signal db_o : cpu_data_i_t;

  signal fl_cs_n  : std_logic;
  signal fl_sck   : std_logic;
  signal fl_io_o  : std_logic_vector(3 downto 0);
  signal fl_io_oe : std_logic_vector(3 downto 0);
  signal fl_io_i  : std_logic_vector(3 downto 0);
  signal model_io_o : std_logic_vector(3 downto 0);

  signal test_done : boolean := false;

  -- transaction counter: count CS falling edges (a new flash burst)
  signal cs_fall_count : natural := 0;

  function byte_of_addr(addr : std_logic_vector(23 downto 0)) return std_logic_vector is
  begin
    return addr(7 downto 0) xor addr(15 downto 8);
  end function;

  function expected_word(addr : std_logic_vector(23 downto 0)) return std_logic_vector is
    variable w : std_logic_vector(31 downto 0);
    variable a : unsigned(23 downto 0);
  begin
    a := unsigned(addr);
    w(31 downto 24) := byte_of_addr(std_logic_vector(a));
    w(23 downto 16) := byte_of_addr(std_logic_vector(a + 1));
    w(15 downto  8) := byte_of_addr(std_logic_vector(a + 2));
    w( 7 downto  0) := byte_of_addr(std_logic_vector(a + 3));
    return w;
  end function;

  procedure bus_read(
    signal clk_s  : in  std_logic;
    signal db_i_s : out cpu_data_o_t;
    signal db_o_s : in  cpu_data_i_t;
    addr          : in  std_logic_vector(31 downto 0);
    variable data : out std_logic_vector(31 downto 0)) is
  begin
    db_i_s.en <= '1';
    db_i_s.rd <= '1';
    db_i_s.wr <= '0';
    db_i_s.a  <= addr;
    wait until rising_edge(clk_s) and db_o_s.ack = '1';
    data := db_o_s.d;
    db_i_s.en <= '0';
    db_i_s.rd <= '0';
    wait until rising_edge(clk_s);
  end procedure;

begin

  clk <= not clk after CLK_PERIOD / 2 when not test_done else clk;

  dut : entity work.qspi_flash_ctrl
    generic map (LANES => 4, DUMMY_CYCLES => 6, FLASH_BASE => FLASH_BASE)
    port map (
      clk      => clk,
      rst      => rst,
      db_i     => db_i,
      db_o     => db_o,
      fl_cs_n  => fl_cs_n,
      fl_sck   => fl_sck,
      fl_io_o  => fl_io_o,
      fl_io_oe => fl_io_oe,
      fl_io_i  => fl_io_i);

  model : entity work.qspi_flash_model
    port map (
      cs_n  => fl_cs_n,
      sck   => fl_sck,
      io_i  => fl_io_o,
      io_oe => fl_io_oe,
      io_o  => model_io_o);

  gen_io : for i in 0 to 3 generate
    fl_io_i(i) <= model_io_o(i) when fl_io_oe(i) = '0' else fl_io_o(i);
  end generate;

  count_p : process (fl_cs_n)
  begin
    if falling_edge(fl_cs_n) then
      cs_fall_count <= cs_fall_count + 1;
    end if;
  end process;

  stim : process
    variable got  : std_logic_vector(31 downto 0);
    variable exp  : std_logic_vector(31 downto 0);
    variable addr : std_logic_vector(31 downto 0);
    variable cnt_before : natural;
  begin
    wait for CLK_PERIOD * 4;
    rst <= '0';
    wait for CLK_PERIOD * 4;

    ---------------------------------------------------------------------
    -- (a) SEQUENTIAL burst of 20 words, starting mid-line (word idx 4
    -- of line 0, byte offset 16), crossing two line boundaries
    -- (at byte 32 and byte 64) -- proves ping-pong swap + prefetch.
    ---------------------------------------------------------------------
    for i in 0 to 19 loop
      addr := std_logic_vector(to_unsigned(16 + 4*i, 32));
      bus_read(clk, db_i, db_o, addr, got);
      exp := expected_word(addr(23 downto 0));
      assert got = exp
        report "pattern (a) sequential: mismatch at addr " &
               integer'image(16 + 4*i) & " got=" & integer'image(to_integer(unsigned(got))) &
               " exp=" & integer'image(to_integer(unsigned(exp))) severity failure;
    end loop;
    report "pattern (a) sequential burst (cross-boundary) PASSED";

    ---------------------------------------------------------------------
    -- (b) RANDOM reads to 4 scattered addresses in different lines.
    ---------------------------------------------------------------------
    for i in 0 to 3 loop
      case i is
        when 0 => addr := x"00000000";
        when 1 => addr := x"00002000";
        when 2 => addr := x"00000100";
        when others => addr := x"00005040";
      end case;
      bus_read(clk, db_i, db_o, addr, got);
      exp := expected_word(addr(23 downto 0));
      assert got = exp
        report "pattern (b) random: mismatch at addr " &
               integer'image(to_integer(unsigned(addr))) severity failure;
    end loop;
    report "pattern (b) random scattered reads PASSED";

    ---------------------------------------------------------------------
    -- (c) Re-read words within an already-buffered line: after priming
    -- the buffer with one read, further reads to the SAME line must not
    -- trigger a new flash transaction (no new CS falling edge).
    ---------------------------------------------------------------------
    addr := x"00009000"; -- fresh, not-yet-buffered line
    bus_read(clk, db_i, db_o, addr, got); -- primes the buffer (1 transaction)
    exp := expected_word(addr(23 downto 0));
    assert got = exp report "pattern (c) prime read mismatch" severity failure;

    wait for CLK_PERIOD * 2; -- let any background prefetch settle
    cnt_before := cs_fall_count;

    for i in 0 to 3 loop
      addr := std_logic_vector(to_unsigned(16#9000# + 4*i, 32));
      bus_read(clk, db_i, db_o, addr, got);
      exp := expected_word(addr(23 downto 0));
      assert got = exp
        report "pattern (c) re-read mismatch at addr " &
               integer'image(16#9000# + 4*i) severity failure;
    end loop;

    assert cs_fall_count = cnt_before
      report "pattern (c): buffer hit triggered an unexpected new flash transaction (cs_fall_count went from " &
             integer'image(cnt_before) & " to " & integer'image(cs_fall_count) & ")"
      severity failure;
    report "pattern (c) buffer-hit-no-retransact PASSED";

    report "PASSED";
    test_done <= true;
    wait;
  end process;

end architecture;
