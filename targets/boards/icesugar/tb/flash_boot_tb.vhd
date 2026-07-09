library ieee;
use ieee.std_logic_1164.all;
use ieee.numeric_std.all;

-- Standalone testbench for flash_boot_reader: drives it against a behavioral
-- SPI-flash slave model (Fast-Read 0x0B, MSB-first, mode 0) and a behavioral
-- SPRAM write-port capture, then asserts the streamed payload round-trips a
-- known pattern (word i = 0xA0000000 + i) into the modelled SPRAM.
--
-- NOTE: ice_spi_io is bypassed here. Its pad side uses inout SB_IO
-- primitives; the only SB_IO sim model in the tree
-- (components/emac/sb_io_sim.vhd) only models an unregistered *input* pad
-- (built for the Ethernet LVDS RX case) and does not drive PACKAGE_PIN as an
-- output, so it cannot stand in for ice_spi_io's CS#/SCK/MOSI *output* pads.
-- Building/validating a full inout SB_IO sim model was out of scope for this
-- unit test. flash_boot_reader's d_cs_n/d_sck/d_mosi/d_miso are logically
-- identical to ice_spi_io's d_* side, so wiring them directly to the
-- behavioral flash model below exercises the same protocol/timing.
entity flash_boot_tb is end entity;

architecture sim of flash_boot_tb is
  constant CLK_PER       : time := 83.333 ns;  -- ~12 MHz
  constant PAYLOAD_WORDS : natural := 64;
  constant FLASH_BASE    : std_logic_vector(23 downto 0) := x"100000";

  signal clk : std_logic := '0';
  signal rst : std_logic := '1';
  signal done_sim : boolean := false;

  signal start : std_logic := '0';
  signal busy  : std_logic;
  signal done  : std_logic;

  signal sp_en : std_logic;
  signal sp_we : std_logic_vector(3 downto 0);
  signal sp_a  : std_logic_vector(16 downto 2);
  signal sp_dw : std_logic_vector(31 downto 0);

  signal d_cs_n : std_logic;
  signal d_sck  : std_logic;
  signal d_mosi : std_logic;
  signal d_miso : std_logic := '0';

  -- captured SPRAM image, indexed by word address
  type mem_t is array (0 to PAYLOAD_WORDS - 1) of std_logic_vector(31 downto 0);
  signal captured_mem : mem_t := (others => (others => '0'));
  signal a0_word0     : std_logic_vector(16 downto 2) := (others => '1'); -- captured sp_a for word 0

  -- watchdog
  constant TIMEOUT_CYCLES : natural := 20000;

begin

  uut : entity work.flash_boot_reader
    generic map (
      FLASH_BASE    => FLASH_BASE,
      PAYLOAD_WORDS => PAYLOAD_WORDS)
    port map (
      clk => clk, rst => rst,
      start => start, busy => busy, done => done,
      sp_en => sp_en, sp_we => sp_we, sp_a => sp_a, sp_dw => sp_dw,
      d_cs_n => d_cs_n, d_sck => d_sck, d_mosi => d_mosi, d_miso => d_miso);

  clk <= not clk after CLK_PER/2 when not done_sim else '0';

  ----------------------------------------------------------------------------
  -- Behavioral SPI-flash slave model: watches cs_n/sck/mosi, decodes a
  -- Fast-Read (0x0B) command + 24-bit address MSB-first, skips 8 dummy
  -- clocks, then shifts out bytes MSB-first from the pattern word i (i =
  -- 0..PAYLOAD_WORDS-1) = 0xA0000000 + i, big-endian byte order. Mode 0:
  -- master drives mosi/sck low->high (sample on rising sck); slave changes
  -- miso on the falling sck edge so it's stable at the master's rising-edge
  -- sample, mirroring what flash_boot_reader (derived from spi_flash_fill)
  -- expects.
  ----------------------------------------------------------------------------
  flash_model : process
    variable cmd_addr    : std_logic_vector(31 downto 0);
    variable addr        : natural;
    variable start_word  : natural;
    variable word_val    : std_logic_vector(31 downto 0);
    variable byte_val    : std_logic_vector(7 downto 0);
  begin
    -- wait for CS to go low (start of a transaction)
    wait until d_cs_n = '0';

    -- shift in 32 bits (cmd+addr) MSB-first, sampling mosi on sck rising edges
    cmd_addr := (others => '0');
    for k in 0 to 31 loop
      wait until rising_edge(d_sck);
      cmd_addr := cmd_addr(30 downto 0) & d_mosi;
    end loop;

    -- 8 dummy clocks
    for k in 0 to 7 loop
      wait until rising_edge(d_sck);
    end loop;

    addr := to_integer(unsigned(cmd_addr(23 downto 0))) - to_integer(unsigned(FLASH_BASE));
    start_word := addr / 4;

    -- stream out words starting at start_word, MSB-first, big-endian byte
    -- order, changing miso on falling sck (so it is stable well before the
    -- next rising edge sample).
    for w in start_word to PAYLOAD_WORDS - 1 loop
      word_val := std_logic_vector(unsigned'(x"A0000000") + to_unsigned(w, 32));
      for b in 0 to 3 loop
        byte_val := word_val((3 - b) * 8 + 7 downto (3 - b) * 8);

        for k in 7 downto 0 loop
          wait until falling_edge(d_sck);
          d_miso <= byte_val(k);
        end loop;
      end loop;
      exit when d_cs_n = '1';
    end loop;

    wait;
  end process;

  ----------------------------------------------------------------------------
  -- Behavioral SPRAM write-port capture
  ----------------------------------------------------------------------------
  capture : process (clk)
  begin
    if rising_edge(clk) then
      if sp_en = '1' and sp_we = "1111" then
        if to_integer(unsigned(sp_a)) < PAYLOAD_WORDS then
          captured_mem(to_integer(unsigned(sp_a))) <= sp_dw;
        end if;
        if to_integer(unsigned(sp_a)) = 0 then
          a0_word0 <= sp_a;
        end if;
      end if;
    end if;
  end process;

  ----------------------------------------------------------------------------
  -- Stimulus + watchdog + checking
  ----------------------------------------------------------------------------
  stim : process
    variable cyc      : natural := 0;
    variable timed_out : boolean := false;
  begin
    rst <= '1';
    wait for CLK_PER * 4;
    wait until rising_edge(clk);
    rst <= '0';
    wait until rising_edge(clk);

    start <= '1';
    wait until rising_edge(clk);
    start <= '0';

    -- watchdog: wait for done, or bail after TIMEOUT_CYCLES clocks
    while done /= '1' and not timed_out loop
      wait until rising_edge(clk);
      cyc := cyc + 1;
      if cyc > TIMEOUT_CYCLES then
        timed_out := true;
      end if;
    end loop;

    if timed_out then
      report "Test Failed: flash_boot_reader never asserted done (timeout)" severity error;
      assert false report "Test Failed" severity failure;
    end if;

    -- let the last SPRAM write settle
    wait until rising_edge(clk);
    wait for 1 ns;

    if a0_word0 /= std_logic_vector(to_unsigned(0, 15)) then
      report "Test Failed: sp_a for word 0 was not 0" severity error;
      assert false report "Test Failed" severity failure;
    end if;

    for i in 0 to PAYLOAD_WORDS - 1 loop
      assert captured_mem(i) = std_logic_vector(unsigned'(x"A0000000") + to_unsigned(i, 32))
        report "Test Failed: word " & integer'image(i) & " mismatch, got " &
               integer'image(to_integer(unsigned(captured_mem(i))))
        severity error;
      if captured_mem(i) /= std_logic_vector(unsigned'(x"A0000000") + to_unsigned(i, 32)) then
        assert false report "Test Failed" severity failure;
      end if;
    end loop;

    report "Test Passed" severity note;

    done_sim <= true;
    wait;
  end process;

end architecture;
