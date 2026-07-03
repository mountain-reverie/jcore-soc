library ieee;
use ieee.std_logic_1164.all;
use ieee.numeric_std.all;
use work.cpu2j0_pack.all;

entity dev_ddr_spram is
  port (
    clk    : in  std_logic;
    ibus_i : in  cpu_instruction_o_t;
    ibus_o : out cpu_instruction_i_t;
    dbus_i : in  cpu_data_o_t;
    dbus_o : out cpu_data_i_t);
end entity;

architecture rtl of dev_ddr_spram is
  signal sp_en : std_logic;
  signal sp_we : std_logic_vector(3 downto 0);
  signal sp_a  : std_logic_vector(16 downto 2);
  signal sp_dw : std_logic_vector(31 downto 0);
  signal sp_dr : std_logic_vector(31 downto 0);

  signal data_go, instr_go : std_logic;      -- who accesses this cycle
  -- registered "in flight" (N+1) response bookkeeping
  signal r_data_ack  : std_logic := '0';
  signal r_instr_ack : std_logic := '0';
  signal r_instr_hi  : std_logic := '0';     -- a(1): pick high 16-bit half
begin
  -- data-priority arbiter
  data_go  <= dbus_i.en;
  instr_go <= ibus_i.en and not dbus_i.en;

  -- drive the single SPRAM port from the winner
  sp_en <= data_go or instr_go;
  sp_we <= dbus_i.we when data_go = '1' else "0000";   -- instr never writes
  sp_a  <= dbus_i.a(16 downto 2) when data_go = '1'
           else ibus_i.a(16 downto 2);
  sp_dw <= dbus_i.d;

  ram : entity work.spram_128k
    port map (clk => clk, en => sp_en, we => sp_we, a => sp_a, dw => sp_dw, dr => sp_dr);

  -- N+1 response bookkeeping
  process (clk) is begin
    if rising_edge(clk) then
      r_data_ack  <= data_go;
      r_instr_ack <= instr_go;
      r_instr_hi  <= ibus_i.a(1);
    end if;
  end process;

  -- data response (full 32 bits)
  dbus_o.d   <= sp_dr;
  dbus_o.ack <= r_data_ack;

  -- instruction response (16-bit half selected by a(1)); big-endian SH-2:
  -- a(1)=0 -> upper half (bits 31:16), a(1)=1 -> lower half (bits 15:0).
  ibus_o.d   <= sp_dr(15 downto 0) when r_instr_hi = '0' else sp_dr(31 downto 16);
  ibus_o.ack <= r_instr_ack;
end architecture;
