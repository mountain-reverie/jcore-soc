library ieee;
use ieee.std_logic_1164.all;

-- ASIC-portable seam: the FSM uses dq_o/dq_oe/dq_i; this wraps the inout dq.
-- ECP5 infers a tristate IO buffer from this idiom.
entity sdram_iocells is
  port (
    dq_o  : in    std_logic_vector(15 downto 0);
    dq_oe : in    std_logic;                       -- '1' = drive dq
    dq_i  : out   std_logic_vector(15 downto 0);
    dq    : inout std_logic_vector(15 downto 0));
end entity;

architecture rtl of sdram_iocells is
begin
  dq   <= dq_o when dq_oe = '1' else (others => 'Z');
  dq_i <= dq;
end architecture;
