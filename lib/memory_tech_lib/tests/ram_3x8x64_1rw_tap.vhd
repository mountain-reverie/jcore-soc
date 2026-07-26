-- Equivalence test for the inferred 64-deep 3x8 1rw RAM primitive
-- (RAM_3x8x64). Elaborates ram_1rw with SUBWORD_WIDTH=8, SUBWORD_NUM=3,
-- ADDR_WIDTH=6 (via the generic ram_1rw_sim configuration, which routes
-- through memory_layout -> genram_3x8x64 -> ram_3x8x64_1rw(sim)) and checks
-- that distinct patterns written to all 64 words read back correctly, and
-- that per-byte write-enables only update the selected byte lanes.
library ieee;
use ieee.std_logic_1164.all;
use ieee.numeric_std.all;

use work.test_pkg.all;
use work.memory_pack.all;

entity ram_3x8x64_1rw_tap is
end entity;

architecture tb of ram_3x8x64_1rw_tap is
  constant DEPTH : integer := 64;

  signal clk : std_logic := '0';
  signal rst : std_logic := '1';

  signal en : std_logic := '0';
  signal wr : std_logic := '0';
  signal we : std_logic_vector(2 downto 0) := "000";
  signal a  : std_logic_vector(5 downto 0) := (others => '0');
  signal dw : std_logic_vector(23 downto 0) := (others => '0');
  signal dr : std_logic_vector(23 downto 0);

  procedure tick(signal clk : inout std_logic) is
  begin
    wait for 10 ns;
    clk <= '1';
    wait for 10 ns;
    clk <= '0';
  end procedure;

  -- distinct 24-bit pattern per address
  function pattern(addr : integer) return std_logic_vector is
    variable v : std_logic_vector(23 downto 0);
  begin
    v := std_logic_vector(to_unsigned(addr, 8)) &
         std_logic_vector(to_unsigned(255 - addr, 8)) &
         std_logic_vector(to_unsigned((addr * 3) mod 256, 8));
    return v;
  end function;

  for dut : ram_1rw use configuration work.ram_1rw_sim;
begin
  dut: ram_1rw
    generic map (
      SUBWORD_WIDTH => 8,
      SUBWORD_NUM => 3,
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
    test_plan(DEPTH + 3, "ram_3x8x64_1rw_tap");

    tick(clk);
    rst <= '0';
    tick(clk);

    -- write distinct patterns to all 64 words, exercising all 3 byte lanes
    for i in 0 to DEPTH - 1 loop
      en <= '1';
      wr <= '1';
      we <= "111";
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

    -- exercise per-byte write-enable: rewrite word 0 with all-ones data but
    -- only enable byte lane 0 (we = "001"); bytes 1 and 2 must be unchanged.
    en <= '1';
    wr <= '1';
    we <= "001";
    a <= std_logic_vector(to_unsigned(0, 6));
    dw <= x"FFFFFF";
    tick(clk);
    en <= '0';
    wr <= '0';
    tick(clk);

    en <= '1';
    wr <= '0';
    a <= std_logic_vector(to_unsigned(0, 6));
    tick(clk);
    test_equal(dr(7 downto 0), x"FF", "byte 0 write-enable updated lane 0");
    test_equal(dr(23 downto 8), pattern(0)(23 downto 8), "byte 0 write-enable left lanes 1,2 unchanged");
    en <= '0';

    -- exercise byte lane 1 write-enable in isolation on word 1
    en <= '1';
    wr <= '1';
    we <= "010";
    a <= std_logic_vector(to_unsigned(1, 6));
    dw <= x"FFFFFF";
    tick(clk);
    en <= '0';
    wr <= '0';
    tick(clk);

    en <= '1';
    wr <= '0';
    a <= std_logic_vector(to_unsigned(1, 6));
    tick(clk);
    test_equal(dr, pattern(1)(23 downto 16) & x"FF" & pattern(1)(7 downto 0),
               "byte 1 write-enable updated only lane 1");
    en <= '0';

    test_finished("done");
    wait;
  end process;
end architecture;
