library ieee;
use ieee.std_logic_1164.all;

-- aic_irq_combine: OR the SMP cross-core IPI interrupt (int0, targeting
-- cpu0's AIC at line 3 -> vector 0x14) into aic0's board-owned irq_i vector.
--
-- ulx3s's aic0.irq_i is bound directly to aic_irq_gen's output (an explicit
-- board mapping, base.yaml), so socgen's normal irqs0-bus auto-wiring is
-- bypassed for aic0 (elaborate/irq.go: aicExplicitIrqI). On the dual-core
-- variants (j2-dual/j4-dual) the ipi device's int0 line still needs to reach
-- aic0, so design.dual-common.yaml renames aic_irq_gen's output to
-- aic_irq_ext and inserts this combiner to fold ipi_i (irqs0(3)) into the
-- final "aic_irq" signal that aic0.irq_i consumes -- everywhere else
-- (buttons/gpio, ext_i bits other than 3) passes through unchanged.
--
-- Not instantiated on single-core variants (j2-direct/j4-rom): those include
-- base.yaml directly, where aic_irq_gen still drives "aic_irq" straight, so
-- they stay byte-identical.
entity aic_irq_combine is
  port (
    ext_i      : in  std_logic_vector(7 downto 0);
    ipi_i      : in  std_logic;
    combined_o : out std_logic_vector(7 downto 0));
end entity;

architecture rtl of aic_irq_combine is
begin
  combined_o(2 downto 0) <= ext_i(2 downto 0);
  combined_o(3)          <= ext_i(3) or ipi_i;
  combined_o(7 downto 4) <= ext_i(7 downto 4);
end architecture;
