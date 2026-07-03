-- Sim-only behavioural model of the iCE40 UP5K SB_SPRAM256KA (16K x 16)
-- single-port RAM. Instantiated as a COMPONENT by spram_128k; at synthesis it
-- is left unbound (--syn-binding) so yosys synth_ice40 maps the real hard block.
-- EXCLUDED from every synth filelist, exactly like components/cpu/core/
-- sb_mac16_sim.vhd. MASKWREN(i) masks nibble i (bits 4i+3..4i). Read is
-- synchronous with 1-cycle latency; on a write cycle DATAOUT returns the OLD
-- word (read-before-write), which is sufficient for this design (the adapter
-- never reads the same address it writes in the same cycle).
library ieee;
use ieee.std_logic_1164.all;
use ieee.numeric_std.all;

entity SB_SPRAM256KA is
  port (
    DATAIN     : in  std_logic_vector(15 downto 0);
    ADDRESS    : in  std_logic_vector(13 downto 0);
    MASKWREN   : in  std_logic_vector(3 downto 0);
    WREN       : in  std_logic;
    CHIPSELECT : in  std_logic;
    CLOCK      : in  std_logic;
    STANDBY    : in  std_logic;
    SLEEP      : in  std_logic;
    POWEROFF   : in  std_logic;
    DATAOUT    : out std_logic_vector(15 downto 0));
end entity;

architecture behave of SB_SPRAM256KA is
  type mem_t is array (0 to 16383) of std_logic_vector(15 downto 0);
  signal mem : mem_t := (others => (others => '0'));
begin
  process (CLOCK) is
    variable idx : integer;
    variable w   : std_logic_vector(15 downto 0);
  begin
    if rising_edge(CLOCK) then
      if CHIPSELECT = '1' then
        idx := to_integer(unsigned(ADDRESS));
        DATAOUT <= mem(idx);                 -- registered read (old value)
        if WREN = '1' then
          w := mem(idx);
          if MASKWREN(0) = '1' then w(3 downto 0)   := DATAIN(3 downto 0);   end if;
          if MASKWREN(1) = '1' then w(7 downto 4)   := DATAIN(7 downto 4);   end if;
          if MASKWREN(2) = '1' then w(11 downto 8)  := DATAIN(11 downto 8);  end if;
          if MASKWREN(3) = '1' then w(15 downto 12) := DATAIN(15 downto 12); end if;
          mem(idx) <= w;
        end if;
      end if;
    end if;
  end process;
end architecture;
