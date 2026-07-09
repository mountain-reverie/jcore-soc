library ieee;
use ieee.std_logic_1164.all;
use ieee.numeric_std.all;
use work.cpu2j0_pack.all;

-- Free-running 32-bit cycle counter, read-only MMIO slave. Never gated by
-- bus activity (increments every clk regardless of accesses); any read of a
-- device-local address returns the current count. Writes are ignored.
entity cycle_counter is
  port (
    clk  : in  std_logic;
    rst  : in  std_logic;
    db_i : in  cpu_data_o_t;
    db_o : out cpu_data_i_t);
end entity;

architecture rtl of cycle_counter is
  signal count : unsigned(31 downto 0) := (others => '0');
  signal ack   : std_logic := '0';
begin
  process (clk, rst) begin
    if rst = '1' then
      count <= (others => '0');
      ack   <= '0';
    elsif rising_edge(clk) then
      count <= count + 1;                    -- free-running, never gated
      ack   <= db_i.en and not ack;           -- one-cycle registered ack
    end if;
  end process;
  db_o.ack <= ack;
  db_o.d   <= std_logic_vector(count);
end architecture;
