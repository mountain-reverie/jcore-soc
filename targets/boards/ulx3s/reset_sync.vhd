library ieee;
use ieee.std_logic_1164.all;

-- reset_sync: async-assert / sync-release reset, extracted from the inline
-- reset process in the hand-written ulx3s_top so socgen can instantiate it as a
-- padring-entity. rst is held '1' while locked='0', then released over two clk
-- edges once the PLL locks.
entity reset_sync is
  port (
    clk    : in  std_logic;
    locked : in  std_logic;
    rst    : out std_logic);
end entity;

architecture rtl of reset_sync is
  signal sync : std_logic_vector(1 downto 0) := "11";
begin
  process(clk, locked)
  begin
    if locked = '0' then
      sync <= "11";
    elsif rising_edge(clk) then
      sync <= sync(0) & '0';
    end if;
  end process;
  rst <= sync(1);
end architecture;
