-------------------------------------------------------------------------------
-- qspi_read_engine_tb.vhd
--
-- Self-check testbench for qspi_read_engine (Task 2): instantiates the
-- engine against the Task-1 qspi_flash_model, pulses start at address
-- 0x000000, waits for done, and checks that line_o holds the 32 bytes
-- byte(0)..byte(31) per the model's deterministic pattern
--   byte(addr) = addr(7 downto 0) xor addr(15 downto 8)
-- in the documented line_o mapping: byte n at
-- line_o(255 - 8*n downto 248 - 8*n) (byte 0 = MS byte of line_o).
--
-- Two DUT instances are checked: LANES=1 (0x0B) and LANES=4 (0xEB). The
-- qspi_flash_model asserts (severity failure) on any io_oe protocol
-- violation, so a clean "PASSED" from this tb also proves the engine's
-- OE sequencing is correct.
-------------------------------------------------------------------------------

library ieee;
use ieee.std_logic_1164.all;
use ieee.numeric_std.all;

entity qspi_read_engine_tb is
end entity;

architecture tb of qspi_read_engine_tb is

  constant CLK_PERIOD : time := 20 ns;

  signal clk : std_logic := '0';
  signal rst : std_logic := '1';

  -- LANES=1 DUT/model wiring
  signal s1_start      : std_logic := '0';
  signal s1_start_addr : std_logic_vector(23 downto 0) := (others => '0');
  signal s1_busy       : std_logic;
  signal s1_done       : std_logic;
  signal s1_line_o     : std_logic_vector(255 downto 0);
  signal s1_line_valid : std_logic;
  signal s1_cs_n       : std_logic;
  signal s1_sck        : std_logic;
  signal s1_io_o       : std_logic_vector(3 downto 0);
  signal s1_io_oe      : std_logic_vector(3 downto 0);
  signal s1_io_i       : std_logic_vector(3 downto 0);
  signal s1_model_io_o : std_logic_vector(3 downto 0);

  -- LANES=4 DUT/model wiring
  signal s4_start      : std_logic := '0';
  signal s4_start_addr : std_logic_vector(23 downto 0) := (others => '0');
  signal s4_busy       : std_logic;
  signal s4_done       : std_logic;
  signal s4_line_o     : std_logic_vector(255 downto 0);
  signal s4_line_valid : std_logic;
  signal s4_cs_n       : std_logic;
  signal s4_sck        : std_logic;
  signal s4_io_o       : std_logic_vector(3 downto 0);
  signal s4_io_oe      : std_logic_vector(3 downto 0);
  signal s4_io_i       : std_logic_vector(3 downto 0);
  signal s4_model_io_o : std_logic_vector(3 downto 0);

  signal test_done : boolean := false;

  function byte_of_addr(addr : std_logic_vector(23 downto 0)) return std_logic_vector is
  begin
    return addr(7 downto 0) xor addr(15 downto 8);
  end function;

begin

  clk <= not clk after CLK_PERIOD / 2 when not test_done else clk;

  -----------------------------------------------------------------------
  -- LANES=1 (0x0B) instance
  -----------------------------------------------------------------------
  dut1 : entity work.qspi_read_engine
    generic map (LANES => 1, DUMMY_CYCLES => 6)
    port map (
      clk        => clk,
      rst        => rst,
      start      => s1_start,
      start_addr => s1_start_addr,
      busy       => s1_busy,
      done       => s1_done,
      line_o     => s1_line_o,
      line_valid => s1_line_valid,
      cs_n       => s1_cs_n,
      sck        => s1_sck,
      io_o       => s1_io_o,
      io_oe      => s1_io_oe,
      io_i       => s1_io_i);

  model1 : entity work.qspi_flash_model
    port map (
      cs_n  => s1_cs_n,
      sck   => s1_sck,
      io_i  => s1_io_o,   -- lines the controller drives
      io_oe => s1_io_oe,
      io_o  => s1_model_io_o);

  -- resolve: engine sees whatever the model drives on lines it isn't
  -- driving itself (model output is 'Z' except when it owns a line).
  gen1 : for i in 0 to 3 generate
    s1_io_i(i) <= s1_model_io_o(i) when s1_io_oe(i) = '0' else s1_io_o(i);
  end generate;

  -----------------------------------------------------------------------
  -- LANES=4 (0xEB) instance
  -----------------------------------------------------------------------
  dut4 : entity work.qspi_read_engine
    generic map (LANES => 4, DUMMY_CYCLES => 6)
    port map (
      clk        => clk,
      rst        => rst,
      start      => s4_start,
      start_addr => s4_start_addr,
      busy       => s4_busy,
      done       => s4_done,
      line_o     => s4_line_o,
      line_valid => s4_line_valid,
      cs_n       => s4_cs_n,
      sck        => s4_sck,
      io_o       => s4_io_o,
      io_oe      => s4_io_oe,
      io_i       => s4_io_i);

  model4 : entity work.qspi_flash_model
    port map (
      cs_n  => s4_cs_n,
      sck   => s4_sck,
      io_i  => s4_io_o,
      io_oe => s4_io_oe,
      io_o  => s4_model_io_o);

  gen4 : for i in 0 to 3 generate
    s4_io_i(i) <= s4_model_io_o(i) when s4_io_oe(i) = '0' else s4_io_o(i);
  end generate;

  -----------------------------------------------------------------------
  -- stimulus
  -----------------------------------------------------------------------
  stim : process
    variable exp : std_logic_vector(7 downto 0);
    variable got : std_logic_vector(7 downto 0);
  begin
    wait for CLK_PERIOD * 4;
    rst <= '0';
    wait for CLK_PERIOD * 4;

    ---------------------------------------------------------------------
    -- LANES=1 (0x0B): fill from address 0x000000
    ---------------------------------------------------------------------
    s1_start_addr <= x"000000";
    s1_start      <= '1';
    wait until rising_edge(clk);
    s1_start      <= '0';

    wait until s1_done = '1' for 200 us;
    assert s1_done = '1'
      report "LANES=1: engine never asserted done" severity failure;

    for n in 0 to 31 loop
      exp := byte_of_addr(std_logic_vector(to_unsigned(n, 24)));
      got := s1_line_o(255 - 8*n downto 248 - 8*n);
      assert got = exp
        report "LANES=1: byte mismatch at index " & integer'image(n)
        severity failure;
    end loop;
    assert s1_line_valid = '1'
      report "LANES=1: line_valid not asserted" severity failure;

    report "LANES=1 PASSED";

    ---------------------------------------------------------------------
    -- LANES=4 (0xEB): fill from address 0x000000
    ---------------------------------------------------------------------
    s4_start_addr <= x"000000";
    s4_start      <= '1';
    wait until rising_edge(clk);
    s4_start      <= '0';

    wait until s4_done = '1' for 200 us;
    assert s4_done = '1'
      report "LANES=4: engine never asserted done" severity failure;

    for n in 0 to 31 loop
      exp := byte_of_addr(std_logic_vector(to_unsigned(n, 24)));
      got := s4_line_o(255 - 8*n downto 248 - 8*n);
      assert got = exp
        report "LANES=4: byte mismatch at index " & integer'image(n)
        severity failure;
    end loop;
    assert s4_line_valid = '1'
      report "LANES=4: line_valid not asserted" severity failure;

    report "LANES=4 PASSED";

    report "PASSED";
    test_done <= true;
    wait;
  end process;

end architecture;
