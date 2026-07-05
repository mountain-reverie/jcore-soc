library ieee;
use ieee.std_logic_1164.all;

-- Adapts the W5500's active-low INTn pin to the AIC's 8-bit rising-edge irq_i
-- bus: bit 0 = not INTn (so a falling INTn -- the W5500 asserting an interrupt
-- -- becomes a rising edge that aic_edgedet latches), all other irq lines 0.
-- Wired as a padring entity (design.yaml) so socgen routes the INTn pad in and
-- the irq vector out to aic0.irq_i, the same way ice_clkgen / ice_spi_io are
-- routed. Trivial combinational glue -- ~0 LC.
entity ice_w5500_irq is
  port (
    int_n : in  std_logic;                      -- W5500 INTn pad (active low)
    irq   : out std_logic_vector(7 downto 0));  -- to aic0.irq_i (bit0 = active-high IRQ)
end entity;

architecture rtl of ice_w5500_irq is
begin
  irq <= "0000000" & (not int_n);
end architecture;
