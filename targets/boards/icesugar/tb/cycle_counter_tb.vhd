library ieee;
use ieee.std_logic_1164.all;
use ieee.numeric_std.all;
use work.cpu2j0_pack.all;

-- Standalone testbench for the free-running cycle_counter MMIO slave.
-- Reset, read c0, wait 50 clocks, read c1, assert c1-c0 in [49,51]
-- (monotonic, ~1 tick/clk), and assert a write does not disturb counting.
entity cycle_counter_tb is end entity;

architecture sim of cycle_counter_tb is
  constant CLK_PER : time := 83.333 ns;  -- ~12 MHz
  signal clk  : std_logic := '0';
  signal rst  : std_logic := '1';
  signal db_i : cpu_data_o_t := NULL_DATA_O;
  signal db_o : cpu_data_i_t;
  signal done : boolean := false;
begin
  uut : entity work.cycle_counter
    port map (clk => clk, rst => rst, db_i => db_i, db_o => db_o);

  clk <= not clk after CLK_PER/2 when not done else '0';

  stim : process
    variable c0, c1, c2 : unsigned(31 downto 0);

    -- Minimal single-cycle-setup bus read: assert en on the entry edge, ack
    -- is registered one cycle later (see cycle_counter.vhd), so the very
    -- next edge (plus a delta settle) has db_o.ack/db_o.d valid.
    procedure do_read(result : out unsigned(31 downto 0)) is
    begin
      db_i.en <= '1';
      db_i.rd <= '1';
      db_i.wr <= '0';
      db_i.a  <= (others => '0');
      wait until rising_edge(clk);
      wait for 1 ns;
      assert db_o.ack = '1' report "cycle_counter_tb: expected ack one cycle after en" severity failure;
      result := unsigned(db_o.d);
      db_i.en <= '0';
      db_i.rd <= '0';
      -- let the registered ack deassert before any following transaction is
      -- allowed to start, otherwise the guarded ack flop would stay stuck.
      wait until rising_edge(clk);
      wait for 1 ns;
      assert db_o.ack = '0' report "cycle_counter_tb: ack did not deassert" severity failure;
    end procedure;

    procedure do_write is
    begin
      db_i.en <= '1';
      db_i.wr <= '1';
      db_i.rd <= '0';
      db_i.we <= "1111";
      db_i.a  <= (others => '0');
      db_i.d  <= x"DEADBEEF";
      wait until rising_edge(clk);
      wait for 1 ns;
      assert db_o.ack = '1' report "cycle_counter_tb: expected ack one cycle after en" severity failure;
      db_i.en <= '0';
      db_i.wr <= '0';
      db_i.we <= "0000";
      wait until rising_edge(clk);
      wait for 1 ns;
      assert db_o.ack = '0' report "cycle_counter_tb: ack did not deassert" severity failure;
    end procedure;
  begin
    -- hold reset for a few cycles then release
    rst <= '1';
    wait for CLK_PER * 4;
    wait until rising_edge(clk);
    rst <= '0';
    wait until rising_edge(clk);

    do_read(c0);

    -- wait ~50 clocks between reads: the explicit loop plus the fixed
    -- 2-cycle overhead of do_read's own ack-deassert wait and the next
    -- do_read's en-to-ack wait bring the total gap to ~50 clocks.
    for i in 1 to 48 loop
      wait until rising_edge(clk);
    end loop;

    do_read(c1);

    -- a write must be ignored (read-only register): it must not disturb
    -- counting, only take a normal ack cycle.
    do_write;

    do_read(c2);

    if (c1 - c0) >= 49 and (c1 - c0) <= 51 and c2 > c1 then
      report "Test Passed" severity note;
    else
      report "Test Failed: c0=" & integer'image(to_integer(c0)) &
             " c1=" & integer'image(to_integer(c1)) &
             " c2=" & integer'image(to_integer(c2)) severity error;
      assert false report "Test Failed" severity failure;
    end if;

    done <= true;
    wait;
  end process;
end architecture;
