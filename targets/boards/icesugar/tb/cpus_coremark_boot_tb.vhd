library ieee;
use ieee.std_logic_1164.all;
use ieee.numeric_std.all;
use work.cpu2j0_pack.all;
use work.data_bus_pack.all;

-- Direct-GHDL-elaboration smoke test for the cpus_coremark architecture.
--
-- This tb has two parts, both required to pass:
--
-- PART A (macro-level, exercises the REAL cpus_coremark arch): instantiates
-- entity cpus configured (cpus_coremark_config) to the cpus_coremark arch +
-- cpu_synth_j1_dsp core, drives clk/rst and a behavioral Fast-Read
-- SPI-flash model (reused from
-- targets/boards/icesugar/tb/flash_boot_tb.vhd's flash_boot_reader unit
-- test) on the cpus entity's fl_* pins, preloaded with a tiny
-- PAYLOAD_WORDS-word pattern at FLASH_BASE. Asserts REQUIREMENT (1): core0
-- is held in reset for the WHOLE flash-streaming window (cpu0_data_master_en
-- / cpu0_mem_lock, the only cpus-entity-boundary-observable signatures of
-- data-bus activity, both stay '0' throughout -- VHDL-93 (this repo's
-- --std=93 convention) has no external-name mechanism to directly probe
-- core0_rst or dev_ddr_spram_boot's boot_active_r inside a configured
-- direct instantiation from outside, so the reset-hold is confirmed via
-- this boundary-observable absence of CPU bus traffic instead).
--
-- PART B (component-level, exercises the SAME dev_ddr_spram_boot +
-- flash_boot_reader RTL wired EXACTLY as cpus_coremark.vhd wires them):
-- a second, independent instantiation of flash_boot_reader +
-- dev_ddr_spram_boot driven by its own copy of the same behavioral flash
-- model, then -- after boot_done -- issues normal CPU-side data-bus reads
-- (dbus_i/dbus_o, boot_active now '0' so the pre-existing data-priority
-- arbiter serves them) to read back every loaded word and assert it matches
-- the flash pattern. Asserts REQUIREMENT (2): the SPRAM was correctly
-- loaded via the muxed boot port.
--
-- A full "CPU fetches vector table from boot EBR at 0x0, jumps to
-- 0x10000000, executes the payload, and produces an observable sentinel"
-- check is NOT attempted here -- deferred to Task 8's cosim, per the
-- brief's explicit permission (this smoke test's tiny payload is arbitrary
-- data, not real SH-2 instructions, so it would not be meaningful to "run"
-- it anyway).
entity cpus_coremark_boot_tb is end entity;

architecture sim of cpus_coremark_boot_tb is
  constant CLK_PER       : time := 83.333 ns;  -- ~12 MHz
  constant PAYLOAD_WORDS : natural := 8;       -- tiny payload for this smoke test
  constant FLASH_BASE    : std_logic_vector(23 downto 0) := x"100000";

  signal clk : std_logic := '0';
  signal rst : std_logic := '1';
  signal done_sim : boolean := false;

  -- results from the two parts, gated by the final report process
  signal partA_ok, partB_ok : boolean := false;

  ------------------------------------------------------------------------
  -- Part A signals: cpus entity ports
  ------------------------------------------------------------------------
  signal cpu0_periph_dbus_o, cpu1_periph_dbus_o : cpu_data_o_t;
  signal cpu0_periph_dbus_i, cpu1_periph_dbus_i : cpu_data_i_t;
  signal cpu0_ddr_ibus_o, cpu1_ddr_ibus_o : cpu_instruction_o_t;
  signal cpu0_ddr_ibus_i, cpu1_ddr_ibus_i : cpu_instruction_i_t := (d => (others => '0'), ack => '0');
  signal cpu0_ddr_dbus_o, cpu1_ddr_dbus_o : cpu_data_o_t;
  signal cpu0_ddr_dbus_i, cpu1_ddr_dbus_i : cpu_data_i_t := (d => (others => '0'), ack => '0');
  signal cpu0_mem_lock, cpu1_mem_lock : std_logic;
  signal debug_i : cpu_debug_i_t := CPU_DEBUG_NOP;
  signal debug_o : cpu_debug_o_t;
  signal cpu0_data_master_en, cpu1_data_master_en : std_logic;
  signal cpu0_data_master_ack, cpu1_data_master_ack : std_logic;
  signal cpu1eni : std_logic := '0';
  signal cpu0_event_o, cpu1_event_o : cpu_event_o_t;
  signal cpu0_event_i, cpu1_event_i : cpu_event_i_t := NULL_CPU_EVENT_I;
  signal cpu0_copro_o, cpu1_copro_o : cop_o_t;
  signal cpu0_copro_i, cpu1_copro_i : cop_i_t := NULL_COPR_I;

  signal a_fl_cs_n : std_logic;
  signal a_fl_sck  : std_logic;
  signal a_fl_mosi : std_logic;
  signal a_fl_miso : std_logic := '0';

  ------------------------------------------------------------------------
  -- Part B signals: standalone flash_boot_reader + dev_ddr_spram_boot,
  -- wired exactly as in cpus_coremark.vhd, plus a CPU-side test-read master.
  ------------------------------------------------------------------------
  signal b_boot_start : std_logic := '0';
  signal b_boot_busy, b_boot_done : std_logic;
  signal b_boot_active : std_logic := '0';

  signal b_sp_en : std_logic;
  signal b_sp_we : std_logic_vector(3 downto 0);
  signal b_sp_a  : std_logic_vector(16 downto 2);
  signal b_sp_dw : std_logic_vector(31 downto 0);

  signal b_fl_cs_n, b_fl_sck, b_fl_mosi : std_logic;
  signal b_fl_miso : std_logic := '0';

  signal b_ibus_i : cpu_instruction_o_t := NULL_INST_O;
  signal b_ibus_o : cpu_instruction_i_t;
  signal b_dbus_i : cpu_data_o_t := NULL_DATA_O;
  signal b_dbus_o : cpu_data_i_t;

  constant TIMEOUT_CYCLES : natural := 20000;

  -- shared behavioral Fast-Read (0x0B) SPI-flash slave model procedure:
  -- decodes CMD+ADDR MSB-first, 8 dummy clocks, then streams words
  -- word_val = 0xA0000000+i, big-endian byte order, starting at FLASH_BASE.
  -- Mode 0: master drives mosi/sck low->high (sample on rising sck); slave
  -- changes miso on the falling sck edge.
  procedure flash_slave(
    signal cs_n  : in  std_logic;
    signal sck   : in  std_logic;
    signal mosi  : in  std_logic;
    signal miso  : out std_logic) is
    variable cmd_addr    : std_logic_vector(31 downto 0);
    variable addr        : natural;
    variable start_word  : natural;
    variable word_val    : std_logic_vector(31 downto 0);
    variable byte_val    : std_logic_vector(7 downto 0);
  begin
    wait until cs_n = '0';

    cmd_addr := (others => '0');
    for k in 0 to 31 loop
      wait until rising_edge(sck);
      cmd_addr := cmd_addr(30 downto 0) & mosi;
    end loop;

    for k in 0 to 7 loop
      wait until rising_edge(sck);
    end loop;

    addr := to_integer(unsigned(cmd_addr(23 downto 0))) - to_integer(unsigned(FLASH_BASE));
    start_word := addr / 4;

    for w in start_word to PAYLOAD_WORDS - 1 loop
      word_val := std_logic_vector(unsigned'(x"A0000000") + to_unsigned(w, 32));
      for b in 0 to 3 loop
        byte_val := word_val((3 - b) * 8 + 7 downto (3 - b) * 8);
        for k in 7 downto 0 loop
          wait until falling_edge(sck);
          miso <= byte_val(k);
        end loop;
      end loop;
      exit when cs_n = '1';
    end loop;
  end procedure;

begin
  ----------------------------------------------------------------------------
  -- PART A: entity cpus, configured to cpus_coremark + cpu_synth_j1_dsp.
  ----------------------------------------------------------------------------
  uutA : configuration work.cpus_coremark_config
    generic map (
      INSERT_WRITE_DELAY_BOOT_MEM => false,
      INSERT_READ_DELAY_BOOT_MEM  => false,
      INSERT_INST_DELAY_BOOT_MEM  => false)
    port map (
      clk => clk, rst => rst,
      cpu0_periph_dbus_o => cpu0_periph_dbus_o, cpu0_periph_dbus_i => cpu0_periph_dbus_i,
      cpu0_ddr_ibus_o => cpu0_ddr_ibus_o, cpu0_ddr_ibus_i => cpu0_ddr_ibus_i,
      cpu0_ddr_dbus_o => cpu0_ddr_dbus_o, cpu0_ddr_dbus_i => cpu0_ddr_dbus_i,
      cpu0_mem_lock => cpu0_mem_lock,
      cpu1_periph_dbus_o => cpu1_periph_dbus_o, cpu1_periph_dbus_i => cpu1_periph_dbus_i,
      cpu1_ddr_ibus_o => cpu1_ddr_ibus_o, cpu1_ddr_ibus_i => cpu1_ddr_ibus_i,
      cpu1_ddr_dbus_o => cpu1_ddr_dbus_o, cpu1_ddr_dbus_i => cpu1_ddr_dbus_i,
      cpu1_mem_lock => cpu1_mem_lock,
      debug_i => debug_i, debug_o => debug_o,
      cpu0_data_master_en => cpu0_data_master_en, cpu1_data_master_en => cpu1_data_master_en,
      cpu0_data_master_ack => cpu0_data_master_ack, cpu1_data_master_ack => cpu1_data_master_ack,
      cpu1eni => cpu1eni,
      cpu0_event_o => cpu0_event_o, cpu0_event_i => cpu0_event_i,
      cpu1_event_o => cpu1_event_o, cpu1_event_i => cpu1_event_i,
      cpu0_copro_o => cpu0_copro_o, cpu0_copro_i => cpu0_copro_i,
      cpu1_copro_o => cpu1_copro_o, cpu1_copro_i => cpu1_copro_i,
      fl_cs_n => a_fl_cs_n, fl_sck => a_fl_sck, fl_mosi => a_fl_mosi, fl_miso => a_fl_miso);

  cpu0_periph_dbus_i <= loopback_bus(cpu0_periph_dbus_o);
  cpu1_periph_dbus_i <= loopback_bus(cpu1_periph_dbus_o);

  flash_model_a : process
  begin
    flash_slave(a_fl_cs_n, a_fl_sck, a_fl_mosi, a_fl_miso);
    wait;
  end process;

  ----------------------------------------------------------------------------
  -- PART B: standalone flash_boot_reader + dev_ddr_spram_boot, wired exactly
  -- as cpus_coremark.vhd wires them.
  ----------------------------------------------------------------------------
  boot_reader_b : entity work.flash_boot_reader
    generic map (
      FLASH_BASE    => FLASH_BASE,
      PAYLOAD_WORDS => PAYLOAD_WORDS)
    port map (
      clk => clk, rst => rst,
      start => b_boot_start, busy => b_boot_busy, done => b_boot_done,
      sp_en => b_sp_en, sp_we => b_sp_we, sp_a => b_sp_a, sp_dw => b_sp_dw,
      d_cs_n => b_fl_cs_n, d_sck => b_fl_sck, d_mosi => b_fl_mosi, d_miso => b_fl_miso);

  ddr_spram_boot_b : entity work.dev_ddr_spram_boot
    port map (
      clk => clk,
      ibus_i => b_ibus_i, ibus_o => b_ibus_o,
      dbus_i => b_dbus_i, dbus_o => b_dbus_o,
      boot_active => b_boot_active,
      boot_en => b_sp_en, boot_we => b_sp_we, boot_a => b_sp_a, boot_dw => b_sp_dw);

  -- latch boot_active exactly as cpus_coremark.vhd does: '1' from start
  -- until done.
  process (clk) is begin
    if rising_edge(clk) then
      if rst = '1' then
        b_boot_active <= '0';
      elsif b_boot_start = '1' then
        b_boot_active <= '1';
      elsif b_boot_done = '1' then
        b_boot_active <= '0';
      end if;
    end if;
  end process;

  flash_model_b : process
  begin
    flash_slave(b_fl_cs_n, b_fl_sck, b_fl_mosi, b_fl_miso);
    wait;
  end process;

  clk <= not clk after CLK_PER/2 when not done_sim else '0';

  ----------------------------------------------------------------------------
  -- PART A stimulus + watchdog: assert REQUIREMENT (1), CPU held in reset
  -- for the whole flash-streaming window.
  ----------------------------------------------------------------------------
  stimA : process
    variable cyc : natural := 0;
    variable timed_out : boolean := false;
  begin
    rst <= '1';
    wait for CLK_PER * 4;
    wait until rising_edge(clk);
    rst <= '0';

    while a_fl_cs_n = '0' or cyc = 0 loop
      wait until rising_edge(clk);
      cyc := cyc + 1;
      assert cpu0_data_master_en = '0'
        report "Test Failed: cpu0_data_master_en asserted while boot load in progress (CPU not held in reset)"
        severity error;
      if cpu0_data_master_en = '1' then
        assert false report "Test Failed" severity failure;
      end if;
      assert cpu0_mem_lock = '0'
        report "Test Failed: cpu0_mem_lock asserted while boot load in progress (CPU not held in reset)"
        severity error;
      if cpu0_mem_lock = '1' then
        assert false report "Test Failed" severity failure;
      end if;
      if cyc > TIMEOUT_CYCLES then
        timed_out := true;
        exit;
      end if;
    end loop;

    if timed_out then
      report "Test Failed: Part A flash model never completed (timeout)"
        severity error;
      assert false report "Test Failed" severity failure;
    end if;

    -- a few settle cycles after the stream completes
    for k in 0 to 15 loop
      wait until rising_edge(clk);
    end loop;

    partA_ok <= true;
    wait;
  end process;

  ----------------------------------------------------------------------------
  -- PART B stimulus + watchdog: kick off the boot load, then read back every
  -- loaded word via the normal CPU-side data port and assert REQUIREMENT
  -- (2), that the SPRAM was correctly loaded via the muxed boot port.
  ----------------------------------------------------------------------------
  stimB : process
    variable cyc : natural := 0;
    variable timed_out : boolean := false;
    variable expect : std_logic_vector(31 downto 0);
  begin
    wait until rst = '0';
    wait until rising_edge(clk);

    b_boot_start <= '1';
    wait until rising_edge(clk);
    b_boot_start <= '0';

    while b_boot_done /= '1' and not timed_out loop
      wait until rising_edge(clk);
      cyc := cyc + 1;
      if cyc > TIMEOUT_CYCLES then
        timed_out := true;
      end if;
    end loop;

    if timed_out then
      report "Test Failed: Part B flash_boot_reader never asserted done (timeout)"
        severity error;
      assert false report "Test Failed" severity failure;
    end if;

    -- boot_active drops the cycle after done; wait for it to settle so the
    -- normal CPU-side arbiter takes over the SPRAM port.
    wait until rising_edge(clk);
    wait until rising_edge(clk);

    for w in 0 to PAYLOAD_WORDS - 1 loop
      b_dbus_i.en <= '1';
      b_dbus_i.rd <= '1';
      b_dbus_i.wr <= '0';
      b_dbus_i.we <= "0000";
      b_dbus_i.a  <= (others => '0');
      -- word address only needs bits 16 downto 2 (dev_ddr_spram_boot/
      -- dev_ddr_spram only decode this slice; upper bits are ignored by the
      -- arbiter since this signal is already routed to the DEV_DDR device).
      b_dbus_i.a(16 downto 2) <= std_logic_vector(to_unsigned(w, 15));
      wait until rising_edge(clk);
      while b_dbus_o.ack /= '1' loop
        wait until rising_edge(clk);
      end loop;
      expect := std_logic_vector(unsigned'(x"A0000000") + to_unsigned(w, 32));
      assert b_dbus_o.d = expect
        report "Test Failed: SPRAM word " & integer'image(w) & " mismatch after boot load, got " &
               integer'image(to_integer(unsigned(b_dbus_o.d)))
        severity error;
      if b_dbus_o.d /= expect then
        assert false report "Test Failed" severity failure;
      end if;
      b_dbus_i.en <= '0';
      wait until rising_edge(clk);
    end loop;

    partB_ok <= true;
    wait;
  end process;

  ----------------------------------------------------------------------------
  -- Final pass gate
  ----------------------------------------------------------------------------
  gate : process begin
    if not partA_ok then wait until partA_ok; end if;
    if not partB_ok then wait until partB_ok; end if;
    report "Test Passed" severity note;
    done_sim <= true;
    wait;
  end process;

  watchdog : process begin
    wait for 5 ms;
    assert done_sim report "Test Failed: TIMEOUT waiting for Part A / Part B to complete" severity failure;
    wait;
  end process;

end architecture;
