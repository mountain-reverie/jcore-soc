library ieee;
use ieee.std_logic_1164.all;

-- Adapts the board's discrete interrupt pins to the AIC's 8-bit rising-edge
-- irq_i bus (aic_edgedet latches a rising edge on each line):
--   irq(0) = not W5500 INTn  (INTn is active low; a falling INTn -- the W5500
--            asserting an interrupt -- becomes a rising edge)
--   irq(1) = DS3231 SQW      (a 1 Hz square wave / alarm: each rising edge is a
--            periodic tick / alarm interrupt)
-- Remaining irq lines are 0. Wired as a padring entity (design.yaml) so socgen
-- routes the pads in and the irq vector out to aic0.irq_i.
entity ice_irq_in is
  port (
    w5500_int_n : in  std_logic;                 -- W5500 INTn pad (active low)
    rtc_sqw     : in  std_logic := '0';          -- DS3231 SQW/INT pad
    irq         : out std_logic_vector(7 downto 0));
end entity;

architecture rtl of ice_irq_in is
begin
  irq <= "000000" & rtc_sqw & (not w5500_int_n);
end architecture;
