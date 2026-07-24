-- Equivalence testbench for the boot RAM (bootram_infer) tech/gf180 vendor
-- macro backend (bootram_2Nx8_gf180.vhd, architecture gf180) against the
-- committed functional-sim/FPGA architecture (inferred).
--
-- WHY a write-then-read pattern (not power-on contents): the vendor macro
-- (unlike (inferred), which is initialised from boot_image_pkg's BOOT_IMAGE
-- array) cannot carry a VHDL power-on init array. So this tb writes a set of
-- known words across SEVERAL depth-tiles (word 0 = tile0, word 512 = tile1,
-- word 3072 = tile6, word 4095 = tile7 -- last word of the last tile) and
-- ALL FOUR byte lanes, then reads them back on both architectures and
-- asserts the two `db_o.d`/`ibus_o.d` streams are bit-identical every cycle.
-- This exercises exactly the logic that matters for a faithful macro:
-- tiling / depth-select decode / byte-lane assembly / arbitration between
-- the data and instruction ports.
--
-- LATENCY: bootram_2Nx8_gf180.vhd clocks its vendor macros from `not clk`,
-- exactly aligning the macro's registered read with (inferred)'s
-- falling-edge register -- so both architectures' outputs are compared on
-- the SAME cycle, no extra latency offset needed (see that file's header
-- for why).
library ieee;
use ieee.std_logic_1164.all;
use ieee.numeric_std.all;
use work.cpu2j0_pack.all;

entity bootram_equiv_tb is end entity;

architecture sim of bootram_equiv_tb is
  signal clk : std_logic := '0';
  signal done : boolean := false;

  signal ibus_i : cpu_instruction_o_t := (en => '0', a => (others => '0'), jp => '0');
  signal ibus_o_inf, ibus_o_gf : cpu_instruction_i_t;
  signal db_i   : cpu_data_o_t := (en => '0', a => (others => '0'), rd => '0', wr => '0', we => "0000", d => (others => '0'));
  signal db_o_inf, db_o_gf : cpu_data_i_t;

  -- when true, compare this cycle's db outputs (both archs, after a db read)
  signal check_db   : boolean := false;
  signal check_ibus : boolean := false;
begin
  uut_inferred : entity work.bootram_infer(inferred)
    generic map (c_addr_width => 14)
    port map (clk => clk, ibus_i => ibus_i, ibus_o => ibus_o_inf, db_i => db_i, db_o => db_o_inf);

  uut_gf180 : entity work.bootram_infer(gf180)
    generic map (c_addr_width => 14)
    port map (clk => clk, ibus_i => ibus_i, ibus_o => ibus_o_gf, db_i => db_i, db_o => db_o_gf);

  clk <= not clk after 20 ns when not done else '0';

  -- Continuous equivalence checkers: whenever a check is armed, the two
  -- archs' outputs must agree on the cycle they're sampled (matching how
  -- the CPU would sample: db_o.ack/ibus_o.ack combinational, data registered).
  check_proc : process(clk)
  begin
    if rising_edge(clk) then
      if check_db then
        assert db_o_inf.ack = db_o_gf.ack
          report "bootram equiv: db ack mismatch" severity failure;
        assert db_o_inf.d = db_o_gf.d
          report "bootram equiv: db data mismatch"
          severity failure;
      end if;
      if check_ibus then
        assert ibus_o_inf.ack = ibus_o_gf.ack
          report "bootram equiv: ibus ack mismatch" severity failure;
        assert ibus_o_inf.d = ibus_o_gf.d
          report "bootram equiv: ibus data mismatch"
          severity failure;
      end if;
    end if;
  end process;

  stim : process
    -- write `word_val` (32-bit) to byte-addressed word index `widx` on both
    -- archs identically (single write driving both instances' shared db_i).
    procedure write_word(widx : integer; word_val : std_logic_vector(31 downto 0)) is
      variable a_bits : std_logic_vector(31 downto 0);
    begin
      a_bits := std_logic_vector(to_unsigned(widx * 4, 32));
      wait until rising_edge(clk);
      db_i.en <= '1'; db_i.a <= a_bits; db_i.wr <= '1'; db_i.we <= "1111"; db_i.d <= word_val;
      wait until rising_edge(clk);
      db_i.en <= '0'; db_i.wr <= '0'; db_i.we <= "0000";
    end procedure;

    -- read word index `widx` via the data port and arm the db checker for
    -- the cycle the data becomes valid.
    procedure read_word_db(widx : integer) is
      variable a_bits : std_logic_vector(31 downto 0);
    begin
      a_bits := std_logic_vector(to_unsigned(widx * 4, 32));
      wait until rising_edge(clk);
      db_i.en <= '1'; db_i.a <= a_bits; db_i.rd <= '1';
      check_db <= true;
      wait until rising_edge(clk);
      check_db <= false;
      db_i.en <= '0'; db_i.rd <= '0';
    end procedure;

    -- instruction-fetch word index `widx`, both halves, arm the ibus checker.
    procedure read_word_ibus(widx : integer) is
      variable a_full : std_logic_vector(31 downto 0);
      variable a_bits : std_logic_vector(31 downto 1);
    begin
      a_full := std_logic_vector(to_unsigned(widx * 4, 32));
      a_bits := a_full(31 downto 1);
      wait until rising_edge(clk);
      ibus_i.en <= '1'; ibus_i.a <= a_bits(31 downto 2) & '1'; -- low half (a(1)='1')
      check_ibus <= true;
      wait until rising_edge(clk);
      check_ibus <= false;
      ibus_i.a <= a_bits(31 downto 2) & '0'; -- high half (a(1)='0')
      check_ibus <= true;
      wait until rising_edge(clk);
      check_ibus <= false;
      ibus_i.en <= '0';
    end procedure;
  begin
    -- Let a couple of idle cycles pass first.
    wait until rising_edge(clk);
    wait until rising_edge(clk);

    -- Write a known pattern across several depth-tiles (0, 1, 6, 7 of 8) and
    -- exercise all four byte lanes via full-word writes with distinct
    -- per-byte values.
    write_word(0,    x"11223344");
    write_word(512,  x"55667788");
    write_word(3072, x"99aabbcc");
    write_word(4095, x"deadbeef");

    -- Read back via the data port -- must match between archs.
    read_word_db(0);
    read_word_db(512);
    read_word_db(3072);
    read_word_db(4095);

    -- Read back via the instruction port (both halves) -- must match.
    read_word_ibus(0);
    read_word_ibus(512);
    read_word_ibus(3072);
    read_word_ibus(4095);

    -- Overwrite a single byte lane and confirm both archs agree post-write.
    wait until rising_edge(clk);
    db_i.en <= '1'; db_i.a <= std_logic_vector(to_unsigned(0, 32)); db_i.wr <= '1';
    db_i.we <= "0010"; db_i.d <= x"1122cc44"; -- byte lane 1 (bits 15:8) = 0xcc; others don't-care (we masks them)
    wait until rising_edge(clk);
    db_i.en <= '0'; db_i.wr <= '0'; db_i.we <= "0000";
    read_word_db(0);

    report "bootram gf180==inferred OK" severity note;
    done <= true;
    wait;
  end process;
end architecture;
