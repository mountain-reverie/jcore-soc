-- Equivalence test for the inferred 512-deep 2x8 2rw RAM primitive
-- (RAM_2x8x512). Elaborates ram_2rw with SUBWORD_WIDTH=8, SUBWORD_NUM=2,
-- ADDR_WIDTH=9 (via the generic ram_2rw_sim configuration, which routes
-- through memory_layout -> genram_2x8x512 -> ram_2x8x512_2rw(sim)) and checks
-- that distinct patterns written via port 0 to all 512 words read back
-- correctly on both port 0 and port 1.
library ieee;
use ieee.std_logic_1164.all;
use ieee.numeric_std.all;

use work.test_pkg.all;
use work.memory_pack.all;

entity ram_2x8x512_2rw_tap is
end entity;

architecture tb of ram_2x8x512_2rw_tap is
  constant DEPTH : integer := 512;

  signal clk0 : std_logic := '0';
  signal rst0 : std_logic := '1';
  signal en0  : std_logic := '0';
  signal wr0  : std_logic := '0';
  signal we0  : std_logic_vector(1 downto 0) := "00";
  signal a0   : std_logic_vector(8 downto 0) := (others => '0');
  signal dw0  : std_logic_vector(15 downto 0) := (others => '0');
  signal dr0  : std_logic_vector(15 downto 0);

  signal clk1 : std_logic := '0';
  signal rst1 : std_logic := '1';
  signal en1  : std_logic := '0';
  signal wr1  : std_logic := '0';
  signal we1  : std_logic_vector(1 downto 0) := "00";
  signal a1   : std_logic_vector(8 downto 0) := (others => '0');
  signal dw1  : std_logic_vector(15 downto 0) := (others => '0');
  signal dr1  : std_logic_vector(15 downto 0);

  procedure tick(signal clk : inout std_logic) is
  begin
    wait for 10 ns;
    clk <= '1';
    wait for 10 ns;
    clk <= '0';
  end procedure;

  -- distinct 16-bit pattern per address
  function pattern(addr : integer) return std_logic_vector is
  begin
    return std_logic_vector(to_unsigned(addr, 9)) &
           std_logic_vector(to_unsigned(511 - addr, 7));
  end function;

  for dut : ram_2rw use configuration work.ram_2rw_sim;
begin
  dut: ram_2rw
    generic map (
      SUBWORD_WIDTH => 8,
      SUBWORD_NUM => 2,
      ADDR_WIDTH => 9)
    port map (
      rst0 => rst0,
      clk0 => clk0,
      en0 => en0,
      wr0 => wr0,
      we0 => we0,
      a0 => a0,
      dw0 => dw0,
      dr0 => dr0,
      rst1 => rst1,
      clk1 => clk1,
      en1 => en1,
      wr1 => wr1,
      we1 => we1,
      a1 => a1,
      dw1 => dw1,
      dr1 => dr1,
      margin0 => '0',
      margin1 => '0');

  process
  begin
    test_plan(2 * DEPTH, "ram_2x8x512_2rw_tap");

    tick(clk0);
    rst0 <= '0';
    rst1 <= '0';
    tick(clk0);

    -- write distinct patterns to all 512 words via port 0
    for i in 0 to DEPTH - 1 loop
      en0 <= '1';
      wr0 <= '1';
      we0 <= "11";
      a0 <= std_logic_vector(to_unsigned(i, 9));
      dw0 <= pattern(i);
      tick(clk0);
    end loop;
    en0 <= '0';
    wr0 <= '0';
    tick(clk0);

    -- read back and verify all 512 words on port 0
    for i in 0 to DEPTH - 1 loop
      en0 <= '1';
      wr0 <= '0';
      a0 <= std_logic_vector(to_unsigned(i, 9));
      tick(clk0);
      test_equal(dr0, pattern(i), "port0 read back word " & integer'image(i));
    end loop;
    en0 <= '0';

    -- read back and verify all 512 words on port 1
    for i in 0 to DEPTH - 1 loop
      en1 <= '1';
      wr1 <= '0';
      a1 <= std_logic_vector(to_unsigned(i, 9));
      tick(clk1);
      test_equal(dr1, pattern(i), "port1 read back word " & integer'image(i));
    end loop;
    en1 <= '0';

    test_finished("done");
    wait;
  end process;
end architecture;
