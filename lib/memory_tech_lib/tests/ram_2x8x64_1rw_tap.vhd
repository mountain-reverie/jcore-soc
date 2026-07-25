-- Equivalence test for the inferred 64-deep 2x8 1rw RAM primitive
-- (RAM_2x8x64). Elaborates ram_1rw with SUBWORD_WIDTH=8, SUBWORD_NUM=2,
-- ADDR_WIDTH=6 (via the generic ram_1rw_sim configuration, which routes
-- through memory_layout -> genram_2x8x64 -> ram_2x8x64_1rw(sim)) and checks
-- that distinct patterns written to all 64 words read back correctly.
library ieee;
use ieee.std_logic_1164.all;
use ieee.numeric_std.all;

use work.test_pkg.all;
use work.memory_pack.all;

entity ram_2x8x64_1rw_tap is
end entity;

architecture tb of ram_2x8x64_1rw_tap is
  constant DEPTH : integer := 64;

  signal clk : std_logic := '0';
  signal rst : std_logic := '1';

  signal en : std_logic := '0';
  signal wr : std_logic := '0';
  signal we : std_logic_vector(1 downto 0) := "00";
  signal a  : std_logic_vector(5 downto 0) := (others => '0');
  signal dw : std_logic_vector(15 downto 0) := (others => '0');
  signal dr : std_logic_vector(15 downto 0);

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
    return std_logic_vector(to_unsigned(addr, 8)) &
           std_logic_vector(to_unsigned(255 - addr, 8));
  end function;

  for dut : ram_1rw use configuration work.ram_1rw_sim;
begin
  dut: ram_1rw
    generic map (
      SUBWORD_WIDTH => 8,
      SUBWORD_NUM => 2,
      ADDR_WIDTH => 6)
    port map (
      rst => rst,
      clk => clk,
      en => en,
      wr => wr,
      we => we,
      a => a,
      dw => dw,
      dr => dr,
      margin => "00");

  process
  begin
    test_plan(DEPTH, "ram_2x8x64_1rw_tap");

    tick(clk);
    rst <= '0';
    tick(clk);

    -- write distinct patterns to all 64 words
    for i in 0 to DEPTH - 1 loop
      en <= '1';
      wr <= '1';
      we <= "11";
      a <= std_logic_vector(to_unsigned(i, 6));
      dw <= pattern(i);
      tick(clk);
    end loop;
    en <= '0';
    wr <= '0';
    tick(clk);

    -- read back and verify all 64 words
    for i in 0 to DEPTH - 1 loop
      en <= '1';
      wr <= '0';
      a <= std_logic_vector(to_unsigned(i, 6));
      tick(clk);
      test_equal(dr, pattern(i), "read back word " & integer'image(i));
    end loop;
    en <= '0';

    test_finished("done");
    wait;
  end process;
end architecture;
