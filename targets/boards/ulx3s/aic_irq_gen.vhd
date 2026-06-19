library ieee;
use ieee.std_logic_1164.all;

-- aic_irq_gen: build the AIC's 8-line irq_i vector from the board's interrupt
-- sources. ULX3S M0 has a single source: button 1 (gpio input pi(1)), which is
-- 2-FF synchronized to clk and presented on irq(0) (the AIC's per-line edge
-- detector turns the held level into an interrupt). Replaces the inline
-- btn1_sync/aic_irq logic of the hand-written ulx3s_top so socgen can
-- instantiate it as a top-entity.
entity aic_irq_gen is
  port (
    clk   : in  std_logic;
    pi_in : in  std_logic_vector(31 downto 0);
    irq   : out std_logic_vector(7 downto 0));
end entity;

architecture rtl of aic_irq_gen is
  signal sync : std_logic_vector(1 downto 0) := "00";
begin
  process(clk)
  begin
    if rising_edge(clk) then
      sync <= sync(0) & pi_in(1);
    end if;
  end process;
  irq <= (0 => sync(1), others => '0');
end architecture;
