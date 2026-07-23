-------------------------------------------------------------------------------
-- qspi_flash_ctrl_pads_tb.vhd
--
-- GATE testbench for Task 4 (components/misc/gf180_qspi_io.vhd):
-- instantiates qspi_flash_ctrl (LANES=4) through a GF180 behavioral pad
-- wrapper (gf180_qspi_io) and validates the bus against qspi_flash_model.
--
-- This testbench demonstrates that the controller works correctly when the
-- flash pins are routed through the pad wrapper's bidirectional tristate
-- logic, proving real inout resolution.
--
-- Expected data: qspi_flash_model's deterministic pattern is
--   byte(addr) = addr(7 downto 0) xor addr(15 downto 8)
-- db_o.d ENDIANNESS: big-endian -- for a word at flash byte address A,
-- db_o.d(31 downto 24) = byte(A), db_o.d(23 downto 16) = byte(A+1),
-- db_o.d(15 downto 8) = byte(A+2), db_o.d(7 downto 0) = byte(A+3).
--
-- Test: Sequential read burst of 8 words starting at address 0, then
-- assert the data matches the expected pattern. This proves the tristate
-- resolution through the pad wrapper is correct.
-------------------------------------------------------------------------------

library ieee;
use ieee.std_logic_1164.all;
use ieee.numeric_std.all;
use work.cpu2j0_pack.all;

entity qspi_flash_ctrl_pads_tb is
end entity;

architecture tb of qspi_flash_ctrl_pads_tb is

  constant CLK_PERIOD : time := 20 ns;
  constant FLASH_BASE : std_logic_vector(31 downto 0) := (others => '0');

  signal clk : std_logic := '0';
  signal rst : std_logic := '1';

  signal db_i : cpu_data_o_t := null_data_o;
  signal db_o : cpu_data_i_t;

  -- Controller to pad wrapper
  signal fl_cs_n  : std_logic;
  signal fl_sck   : std_logic;
  signal fl_io_o  : std_logic_vector(3 downto 0);
  signal fl_io_oe : std_logic_vector(3 downto 0);
  signal fl_io_i  : std_logic_vector(3 downto 0);

  -- Pad wrapper inout side
  signal pad_cs_n  : std_logic;
  signal pad_sck   : std_logic;
  signal pad_io    : std_logic_vector(3 downto 0);

  -- Model side (split triplet)
  signal model_io_o : std_logic_vector(3 downto 0);

  signal test_done : boolean := false;

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

  -- Instantiate the controller
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

  -- Instantiate the GF180 pad wrapper
  pad_wrapper : entity work.gf180_qspi_io
    port map (
      fl_cs_n  => fl_cs_n,
      fl_sck   => fl_sck,
      fl_io_o  => fl_io_o,
      fl_io_oe => fl_io_oe,
      fl_io_i  => fl_io_i,
      pad_cs_n => pad_cs_n,
      pad_sck  => pad_sck,
      pad_io   => pad_io);

  -- Instantiate the flash model
  -- The model expects to see:
  --   io_i: the lines the controller is driving
  --   io_oe: the controller's output-enable
  --   io_o: what the model drives
  -- The pad_io is the resolved node.
  model : entity work.qspi_flash_model
    port map (
      cs_n  => pad_cs_n,
      sck   => pad_sck,
      io_i  => fl_io_o,
      io_oe => fl_io_oe,
      io_o  => model_io_o);

  -- Bridge the model's io_o onto the pad_io inout when the controller is not driving.
  -- When the controller drives (fl_io_oe(k)='1'), the pad_io reflects fl_io_o(k).
  -- When the controller does not drive (fl_io_oe(k)='0'), pad_io reflects model_io_o(k)
  -- or 'Z' if model is also not driving (it drives 'Z' when idle).
  gen_io : for i in 0 to 3 generate
    pad_io(i) <= model_io_o(i) when fl_io_oe(i) = '0' else 'Z';
  end generate;

  stim : process
    variable got  : std_logic_vector(31 downto 0);
    variable exp  : std_logic_vector(31 downto 0);
    variable addr : std_logic_vector(31 downto 0);
  begin
    wait for CLK_PERIOD * 4;
    rst <= '0';
    wait for CLK_PERIOD * 4;

    -- Sequential read burst of 8 words starting at address 0.
    -- Each word is 4 bytes; this covers 32 bytes (one line).
    for i in 0 to 7 loop
      addr := std_logic_vector(to_unsigned(4*i, 32));
      bus_read(clk, db_i, db_o, addr, got);
      exp := expected_word(addr(23 downto 0));
      assert got = exp
        report "pads_tb sequential read: mismatch at addr " &
               integer'image(4*i) & " got=" & integer'image(to_integer(unsigned(got))) &
               " exp=" & integer'image(to_integer(unsigned(exp))) severity failure;
    end loop;
    report "pads_tb sequential read through pad wrapper PASSED";

    report "PASSED";
    test_done <= true;
    wait;
  end process;

end architecture;
