library ieee;
use ieee.std_logic_1164.all;
use ieee.numeric_std.all;
use work.cpu2j0_pack.all;

-- dev_ddr_spram_boot: a copy of dev_ddr_spram with an added boot-time write
-- path. Used by architectures (e.g. icesugar's cpus_coremark) that load the
-- SPRAM contents from an external config flash via flash_boot_reader before
-- releasing the CPU from reset.
--
-- When boot_active='1' the single spram_128k port is driven entirely from
-- the boot_* inputs (the CPU is expected to be held in reset during this
-- time so it issues no requests; as a belt-and-suspenders precaution the
-- CPU-side acks are also gated to 0 while boot_active='1'). When
-- boot_active='0' this is bit-for-bit the existing dev_ddr_spram
-- data-priority arbiter -- unchanged.
--
-- dev_ddr_spram itself is left untouched so other boards are unaffected.
entity dev_ddr_spram_boot is
  port (
    clk    : in  std_logic;
    ibus_i : in  cpu_instruction_o_t;
    ibus_o : out cpu_instruction_i_t;
    dbus_i : in  cpu_data_o_t;
    dbus_o : out cpu_data_i_t;

    -- boot write path (from flash_boot_reader)
    boot_active : in std_logic;
    boot_en     : in std_logic;
    boot_we     : in std_logic_vector(3 downto 0);
    boot_a      : in std_logic_vector(16 downto 2);
    boot_dw     : in std_logic_vector(31 downto 0));
end entity;

architecture rtl of dev_ddr_spram_boot is
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
  -- data-priority arbiter. Each request must produce exactly ONE ack pulse.
  -- The CPU holds en asserted from the cycle it issues a request until the
  -- cycle it samples ack='1' (the actual bus deassertion of en only becomes
  -- visible a cycle later). Without qualifying on "not yet acked", en staying
  -- high during that extra cycle would re-trigger a second RAM access and a
  -- second, spurious ack pulse for the same logical request -- corrupting the
  -- N+1 handshake and confusing whoever is waiting on a single ack. Gate each
  -- go signal so it only fires once per outstanding request (until en drops).
  data_go  <= dbus_i.en and not r_data_ack;
  instr_go <= ibus_i.en and not dbus_i.en and not r_instr_ack;

  -- Mux the single SPRAM port: boot_active gives the boot loader exclusive
  -- ownership; otherwise this is exactly the CPU data-priority arbiter.
  sp_en <= boot_en when boot_active = '1' else (data_go or instr_go);
  sp_we <= boot_we when boot_active = '1' else
           dbus_i.we when data_go = '1' else "0000";   -- instr never writes
  sp_a  <= boot_a when boot_active = '1' else
           dbus_i.a(16 downto 2) when data_go = '1' else ibus_i.a(16 downto 2);
  sp_dw <= boot_dw when boot_active = '1' else dbus_i.d;

  ram : entity work.spram_128k
    port map (clk => clk, en => sp_en, we => sp_we, a => sp_a, dw => sp_dw, dr => sp_dr);

  -- N+1 response bookkeeping. While boot_active='1' the CPU is expected to
  -- be held in reset (no requests), but gate data_go/instr_go's contribution
  -- to the ack pipeline for safety so no spurious CPU-side ack can occur.
  process (clk) is begin
    if rising_edge(clk) then
      if boot_active = '1' then
        r_data_ack  <= '0';
        r_instr_ack <= '0';
      else
        r_data_ack  <= data_go;
        r_instr_ack <= instr_go;
      end if;
      r_instr_hi <= ibus_i.a(1);
    end if;
  end process;

  -- data response (full 32 bits)
  dbus_o.d   <= sp_dr;
  dbus_o.ack <= r_data_ack;

  -- instruction response (16-bit half selected by a(1)); big-endian SH-2:
  -- a(1)=0 -> upper half (bits 31:16), a(1)=1 -> lower half (bits 15:0).
  ibus_o.d   <= sp_dr(31 downto 16) when r_instr_hi = '0' else sp_dr(15 downto 0);
  ibus_o.ack <= r_instr_ack;
end architecture;
