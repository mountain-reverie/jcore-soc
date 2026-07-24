-------------------------------------------------------------------------------
-- qspi_flash_ctrl_burst_tb.vhd
--
-- Task 1 (XIP sub-project) burst tb: instantiates qspi_flash_ctrl (LANES=4)
-- against the Task-1 qspi_flash_model and drives it as a BURST master
-- following the sdram_ctrl.vhd req/bst/resp/ack_r contract -- asserting
-- bst='1' + a line-aligned address, then waiting each ack_r and updating
-- the (held) address by +4 per ack, capturing 8 words per burst.
--
-- Expected data: qspi_flash_model's deterministic pattern is
--   byte(addr) = addr(7 downto 0) xor addr(15 downto 8)
-- db_o.d ENDIANNESS (documented in qspi_flash_ctrl.vhd): big-endian -- for
-- a word at flash byte address A, db_o.d(31 downto 24) = byte(A),
-- db_o.d(23 downto 16) = byte(A+1), db_o.d(15 downto 8) = byte(A+2),
-- db_o.d(7 downto 0) = byte(A+3).
--
-- Coverage:
--   (a) COLD burst: a line never touched before -> miss -> QSPI fill ->
--       8-beat stream. Verifies all 8 words of the line.
--   (b) BUFFERED burst: burst-read a DIFFERENT already-buffered/prefetched
--       line (primed by a preceding single-word read, which also fires the
--       sequential-prefetch path) -- proves the HIT burst path (no new
--       flash transaction) via a CS falling-edge count check.
--   (c) SINGLE-word path (bst='0') still works after burst traffic --
--       proves the single-word path is unaffected.
--   (d)/(e) CRITICAL-WORD-FIRST regression guard: burst a line at a
--       NON-zero requested word offset within the 32-byte line (widx=5,
--       then widx=1) and assert the 8 beats arrive
--       word[(widx+beat) mod 8] -- i.e. the requested word FIRST, then
--       wrapping through the rest of the line -- matching sdram_ctrl's
--       critical-word-first delivery order that dcache_mcl assumes. This
--       is the exact regression guarded against: qspi_flash_ctrl used to
--       always start burst delivery at word 0 regardless of the requested
--       word index (see components/misc/qspi_flash_ctrl.vhd's bst_widx
--       fix, commit 97c52b1 / .superpowers/sdd/task-4-report.md).
-------------------------------------------------------------------------------

library ieee;
use ieee.std_logic_1164.all;
use ieee.numeric_std.all;
use work.cpu2j0_pack.all;

entity qspi_flash_ctrl_burst_tb is
end entity;

architecture tb of qspi_flash_ctrl_burst_tb is

  constant CLK_PERIOD : time := 20 ns;
  constant FLASH_BASE : std_logic_vector(31 downto 0) := (others => '0');

  signal clk : std_logic := '0';
  signal rst : std_logic := '1';

  signal db_i  : cpu_data_o_t := null_data_o;
  signal bst   : std_logic := '0';
  signal db_o  : cpu_data_i_t;
  signal ack_r : std_logic;

  signal fl_cs_n  : std_logic;
  signal fl_sck   : std_logic;
  signal fl_io_o  : std_logic_vector(3 downto 0);
  signal fl_io_oe : std_logic_vector(3 downto 0);
  signal fl_io_i  : std_logic_vector(3 downto 0);
  signal model_io_o : std_logic_vector(3 downto 0);

  signal test_done : boolean := false;

  -- transaction counter: count CS falling edges (a new flash burst)
  signal cs_fall_count : natural := 0;

  type words8_t is array (0 to 7) of std_logic_vector(31 downto 0);

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

  -- single-word bus_read (bst='0'), per the SP1 protocol -- unchanged.
  procedure bus_read(
    signal clk_s  : in  std_logic;
    signal db_i_s : out cpu_data_o_t;
    signal bst_s  : out std_logic;
    signal db_o_s : in  cpu_data_i_t;
    addr          : in  std_logic_vector(31 downto 0);
    variable data : out std_logic_vector(31 downto 0)) is
  begin
    db_i_s.en <= '1';
    db_i_s.rd <= '1';
    db_i_s.wr <= '0';
    db_i_s.a  <= addr;
    bst_s     <= '0';
    wait until rising_edge(clk_s) and db_o_s.ack = '1';
    data := db_o_s.d;
    db_i_s.en <= '0';
    db_i_s.rd <= '0';
    wait until rising_edge(clk_s);
  end procedure;

  -- burst-master procedure: mirrors sdram_ctrl.vhd's req/bst/resp/ack_r
  -- contract. Holds en/rd/bst, presents the line-aligned start address,
  -- then for each of the 8 beats waits for ack_r, captures resp.d, and
  -- updates (holds) req.a to the next word address -- the per-ack address
  -- update convention the requester follows.
  procedure bus_burst_read(
    signal clk_s   : in  std_logic;
    signal db_i_s  : out cpu_data_o_t;
    signal bst_s   : out std_logic;
    signal db_o_s  : in  cpu_data_i_t;
    signal ack_r_s : in  std_logic;
    addr           : in  std_logic_vector(31 downto 0);
    variable data  : out words8_t) is
    variable a : unsigned(31 downto 0);
  begin
    a := unsigned(addr);
    db_i_s.en <= '1';
    db_i_s.rd <= '1';
    db_i_s.wr <= '0';
    db_i_s.a  <= std_logic_vector(a);
    bst_s     <= '1';

    for i in 0 to 7 loop
      wait until rising_edge(clk_s) and db_o_s.ack = '1' and ack_r_s = '1';
      data(i) := db_o_s.d;
      a := a + 4;
      db_i_s.a <= std_logic_vector(a); -- per-ack address update (master convention)
    end loop;

    db_i_s.en <= '0';
    db_i_s.rd <= '0';
    bst_s     <= '0';
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
      bst      => bst,
      db_o     => db_o,
      ack_r    => ack_r,
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
    variable got8  : words8_t;
    variable got   : std_logic_vector(31 downto 0);
    variable exp   : std_logic_vector(31 downto 0);
    variable addr  : std_logic_vector(31 downto 0);
    variable la    : unsigned(31 downto 0);
    variable wa    : unsigned(31 downto 0);
    variable cnt_before : natural;
  begin
    wait for CLK_PERIOD * 4;
    rst <= '0';
    wait for CLK_PERIOD * 4;

    ---------------------------------------------------------------------
    -- (a) COLD burst: line never touched -> miss -> QSPI fill -> 8-beat
    -- stream. Verify all 8 words of the line.
    ---------------------------------------------------------------------
    addr := x"00003000"; -- fresh, line-aligned, not-yet-buffered
    bus_burst_read(clk, db_i, bst, db_o, ack_r, addr, got8);
    la := unsigned(addr(31 downto 0));
    for i in 0 to 7 loop
      exp := expected_word(std_logic_vector(la(23 downto 0)));
      assert got8(i) = exp
        report "pattern (a) cold burst: mismatch at word " & integer'image(i) &
               " got=" & integer'image(to_integer(unsigned(got8(i)))) &
               " exp=" & integer'image(to_integer(unsigned(exp))) severity failure;
      la := la + 4;
    end loop;
    report "pattern (a) cold burst (miss -> fill -> 8 beats) PASSED";

    ---------------------------------------------------------------------
    -- (b) BUFFERED burst: prime a DIFFERENT line with a single-word read
    -- (1 transaction), then read its LAST word (widx=7) to also fire the
    -- SP1 sequential run-ahead prefetch of the FOLLOWING line (1 more
    -- transaction) -- exactly what a burst covering the same line would
    -- also trigger at wcnt=6/7 (the burst response mirrors the same
    -- prefetch check, per qspi_flash_ctrl.vhd). Wait for that prefetch to
    -- complete, then burst-read the primed line: since both it AND the
    -- line after it are now buffered, the burst must cause NO further
    -- flash transaction.
    ---------------------------------------------------------------------
    addr := x"00004000"; -- fresh line, distinct from (a)
    bus_read(clk, db_i, bst, db_o, addr, got); -- primes buffer (1 transaction)
    exp := expected_word(addr(23 downto 0));
    assert got = exp report "pattern (b) prime read mismatch" severity failure;

    bus_read(clk, db_i, bst, db_o, x"0000401C", got); -- widx=7 -> fires prefetch (1 more transaction)
    exp := expected_word(x"00401C");
    assert got = exp report "pattern (b) prefetch-trigger read mismatch" severity failure;

    wait for CLK_PERIOD * 250; -- let the background prefetch fill fully complete
    cnt_before := cs_fall_count;

    bus_burst_read(clk, db_i, bst, db_o, ack_r, addr, got8);
    la := unsigned(addr(31 downto 0));
    for i in 0 to 7 loop
      exp := expected_word(std_logic_vector(la(23 downto 0)));
      assert got8(i) = exp
        report "pattern (b) buffered burst: mismatch at word " & integer'image(i) &
               " got=" & integer'image(to_integer(unsigned(got8(i)))) &
               " exp=" & integer'image(to_integer(unsigned(exp))) severity failure;
      la := la + 4;
    end loop;

    assert cs_fall_count = cnt_before
      report "pattern (b): buffered burst triggered an unexpected new flash transaction (cs_fall_count went from " &
             integer'image(cnt_before) & " to " & integer'image(cs_fall_count) & ")"
      severity failure;
    report "pattern (b) buffered burst (HIT, no retransact) PASSED";

    ---------------------------------------------------------------------
    -- (c) SINGLE-word path (bst='0') still works after burst traffic.
    ---------------------------------------------------------------------
    addr := x"00006000"; -- fresh line, distinct from (a)/(b)
    bus_read(clk, db_i, bst, db_o, addr, got);
    exp := expected_word(addr(23 downto 0));
    assert got = exp report "pattern (c) single-word mismatch" severity failure;

    addr := x"00006004";
    bus_read(clk, db_i, bst, db_o, addr, got);
    exp := expected_word(addr(23 downto 0));
    assert got = exp report "pattern (c) single-word (2nd) mismatch" severity failure;
    report "pattern (c) single-word path intact PASSED";

    ---------------------------------------------------------------------
    -- (d) CRITICAL-WORD-FIRST: burst requested at word index 5 (address =
    -- line_base + 5*4) within a fresh line. Beat i must deliver word
    -- (5 + i) mod 8, NOT word i directly.
    ---------------------------------------------------------------------
    addr := x"00007014"; -- fresh line (base 0x7000), word index 5 (0x14 = 5*4)
    bus_burst_read(clk, db_i, bst, db_o, ack_r, addr, got8);
    la := unsigned(addr(31 downto 0)) and x"FFFFFFE0"; -- line base
    for i in 0 to 7 loop
      wa  := la + to_unsigned(((5 + i) mod 8) * 4, 32);
      exp := expected_word(std_logic_vector(wa(23 downto 0)));
      assert got8(i) = exp
        report "pattern (d) critical-word-first (widx=5): mismatch at beat " & integer'image(i) &
               " got=" & integer'image(to_integer(unsigned(got8(i)))) &
               " exp=" & integer'image(to_integer(unsigned(exp))) severity failure;
    end loop;
    report "pattern (d) critical-word-first burst (widx=5) PASSED";

    ---------------------------------------------------------------------
    -- (e) CRITICAL-WORD-FIRST: second, cheaper non-zero offset (widx=1)
    -- on another fresh line, for extra confidence beyond a single offset.
    ---------------------------------------------------------------------
    addr := x"00008004"; -- fresh line (base 0x8000), word index 1 (0x04 = 1*4)
    bus_burst_read(clk, db_i, bst, db_o, ack_r, addr, got8);
    la := unsigned(addr(31 downto 0)) and x"FFFFFFE0"; -- line base
    for i in 0 to 7 loop
      wa  := la + to_unsigned(((1 + i) mod 8) * 4, 32);
      exp := expected_word(std_logic_vector(wa(23 downto 0)));
      assert got8(i) = exp
        report "pattern (e) critical-word-first (widx=1): mismatch at beat " & integer'image(i) &
               " got=" & integer'image(to_integer(unsigned(got8(i)))) &
               " exp=" & integer'image(to_integer(unsigned(exp))) severity failure;
    end loop;
    report "pattern (e) critical-word-first burst (widx=1) PASSED";

    report "PASSED";
    test_done <= true;
    wait;
  end process;

end architecture;
