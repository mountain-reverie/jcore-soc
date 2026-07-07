library ieee;
use ieee.std_logic_1164.all;
use ieee.numeric_std.all;
use work.cpu2j0_pack.all;
use work.data_bus_pack.all;
use work.cpu_core_pack.all;

-- Task 5/6 smoke test: elaborates cpus(one_cpu_xip) (the CPU + spi_page_cache
-- + DEV_DDR window-mux + fault-sideband architecture) directly (not through
-- the full board pad_ring/soc.vhd -- that is Task 7's integration level) and
-- runs reset + free-running clock for >=10 us, asserting only that
-- instruction fetch activity is observed (elaboration is clean and the CPU
-- isn't wedged) -- i.e. "no bus error". A behavioral SPI flash model
-- (flash_mem(k) = k mod 256, same convention as spi_page_cache_tb) is wired
-- up in case any accidental fill/config-flash access occurs, but note: the
-- generated `cpus` entity (targets/cpus.vhd) has no flash pin ports, so
-- spi_page_cache's SPI pins are NOT reachable from outside cpus_xip.vhd (they
-- terminate on ice_spi_io's pin_* signals, internal to that architecture).
-- This flash model is therefore inert for this smoke test; a real, reachable
-- flash model requires Task 8's padring work to expose those pins on `cpus`
-- (or the full board top). Full self-check (fill + hit + MMIO + fault
-- servicing over the real reset vector) is Task 7.
--
-- All non-DEV_DDR/non-DEV_PERIPH master-side inputs this smoke test doesn't
-- otherwise drive (cpu0_ddr_ibus_i/dbus_i, cpu1_*, debug_i, event_i, copro_i)
-- are tied to loopback/NULL defaults -- cpus_xip.vhd itself ties off
-- cpu0_ddr_ibus_o/dbus_o (no external DDR on iCESugar) and all cpu1_* outputs,
-- so those inputs are simply unused/don't-care from the DUT's perspective.
entity cpus_xip_tb is end entity;

architecture sim of cpus_xip_tb is
  constant CLK_PER : time := 20 ns; -- ~50 MHz free-running sim clock

  signal clk : std_logic := '0';
  signal rst : std_logic := '1';
  signal done : boolean := false;

  signal cpu0_periph_dbus_o : cpu_data_o_t;
  signal cpu0_periph_dbus_i : cpu_data_i_t;
  signal cpu0_ddr_ibus_o : cpu_instruction_o_t;
  signal cpu0_ddr_ibus_i : cpu_instruction_i_t;
  signal cpu0_ddr_dbus_o : cpu_data_o_t;
  signal cpu0_ddr_dbus_i : cpu_data_i_t;
  signal cpu0_mem_lock : std_logic;

  signal cpu1_periph_dbus_o : cpu_data_o_t;
  signal cpu1_periph_dbus_i : cpu_data_i_t := (d => (others => '0'), ack => '0');
  signal cpu1_ddr_ibus_o : cpu_instruction_o_t;
  signal cpu1_ddr_ibus_i : cpu_instruction_i_t := (d => (others => '0'), ack => '0');
  signal cpu1_ddr_dbus_o : cpu_data_o_t;
  signal cpu1_ddr_dbus_i : cpu_data_i_t := (d => (others => '0'), ack => '0');
  signal cpu1_mem_lock : std_logic;

  signal debug_o : cpu_debug_o_t;
  signal cpu0_data_master_en : std_logic;
  signal cpu1_data_master_en : std_logic;
  signal cpu0_data_master_ack : std_logic;
  signal cpu1_data_master_ack : std_logic;

  signal cpu0_event_o : cpu_event_o_t;
  signal cpu1_event_o : cpu_event_o_t;

  signal cpu0_copro_o : cop_o_t;
  signal cpu1_copro_o : cop_o_t;

  signal fetch_seen : boolean := false;

  component cpus is
    generic (
      INSERT_WRITE_DELAY_BOOT_MEM : boolean;
      INSERT_READ_DELAY_BOOT_MEM : boolean;
      INSERT_INST_DELAY_BOOT_MEM : boolean);
    port (
      clk : in std_logic;
      rst : in std_logic;
      cpu0_periph_dbus_o : out cpu_data_o_t;
      cpu0_periph_dbus_i : in  cpu_data_i_t;
      cpu0_ddr_ibus_o : out cpu_instruction_o_t;
      cpu0_ddr_ibus_i : in  cpu_instruction_i_t;
      cpu0_ddr_dbus_o : out cpu_data_o_t;
      cpu0_ddr_dbus_i : in cpu_data_i_t;
      cpu0_mem_lock : out std_logic;
      cpu1_periph_dbus_o : out cpu_data_o_t;
      cpu1_periph_dbus_i : in cpu_data_i_t;
      cpu1_ddr_ibus_o : out cpu_instruction_o_t;
      cpu1_ddr_ibus_i : in  cpu_instruction_i_t;
      cpu1_ddr_dbus_o : out cpu_data_o_t;
      cpu1_ddr_dbus_i : in  cpu_data_i_t;
      cpu1_mem_lock : out std_logic;
      debug_i : in  cpu_debug_i_t;
      debug_o : out cpu_debug_o_t;
      cpu0_data_master_en : out std_logic;
      cpu1_data_master_en : out std_logic;
      cpu0_data_master_ack : out std_logic;
      cpu1_data_master_ack : out std_logic;
      cpu1eni : in std_logic;
      cpu0_event_o : out cpu_event_o_t;
      cpu0_event_i : in cpu_event_i_t;
      cpu1_event_o : out cpu_event_o_t;
      cpu1_event_i : in cpu_event_i_t;
      cpu0_copro_o : out cop_o_t;
      cpu0_copro_i : in  cop_i_t;
      cpu1_copro_o : out cop_o_t;
      cpu1_copro_i : in  cop_i_t);
  end component;

begin

  clk <= not clk after CLK_PER / 2 when not done else '0';

  -- Loop back the buses nothing external drives in this smoke test (matches
  -- what devices.vhd / an unpopulated external DDR would present): DEV_PERIPH
  -- and DEV_DDR (external) masters get an ack'd, all-zero-data reply so an
  -- incidental access doesn't wedge the CPU.
  cpu0_periph_dbus_i <= loopback_bus(cpu0_periph_dbus_o);
  cpu0_ddr_ibus_i <= loopback_bus(cpu0_ddr_ibus_o);
  cpu0_ddr_dbus_i <= loopback_bus(cpu0_ddr_dbus_o);

  dut : cpus
    generic map (
      INSERT_WRITE_DELAY_BOOT_MEM => false,
      INSERT_READ_DELAY_BOOT_MEM => false,
      INSERT_INST_DELAY_BOOT_MEM => false)
    port map (
      clk => clk, rst => rst,
      cpu0_periph_dbus_o => cpu0_periph_dbus_o, cpu0_periph_dbus_i => cpu0_periph_dbus_i,
      cpu0_ddr_ibus_o => cpu0_ddr_ibus_o, cpu0_ddr_ibus_i => cpu0_ddr_ibus_i,
      cpu0_ddr_dbus_o => cpu0_ddr_dbus_o, cpu0_ddr_dbus_i => cpu0_ddr_dbus_i,
      cpu0_mem_lock => cpu0_mem_lock,
      cpu1_periph_dbus_o => cpu1_periph_dbus_o, cpu1_periph_dbus_i => cpu1_periph_dbus_i,
      cpu1_ddr_ibus_o => cpu1_ddr_ibus_o, cpu1_ddr_ibus_i => cpu1_ddr_ibus_i,
      cpu1_ddr_dbus_o => cpu1_ddr_dbus_o, cpu1_ddr_dbus_i => cpu1_ddr_dbus_i,
      cpu1_mem_lock => cpu1_mem_lock,
      debug_i => CPU_DEBUG_NOP, debug_o => debug_o,
      cpu0_data_master_en => cpu0_data_master_en, cpu1_data_master_en => cpu1_data_master_en,
      cpu0_data_master_ack => cpu0_data_master_ack, cpu1_data_master_ack => cpu1_data_master_ack,
      cpu1eni => '0',
      cpu0_event_o => cpu0_event_o, cpu0_event_i => NULL_CPU_EVENT_I,
      cpu1_event_o => cpu1_event_o, cpu1_event_i => NULL_CPU_EVENT_I,
      cpu0_copro_o => cpu0_copro_o, cpu0_copro_i => NULL_COPR_I,
      cpu1_copro_o => cpu1_copro_o, cpu1_copro_i => NULL_COPR_I);

  smoke : process
  begin
    rst <= '1';
    wait for CLK_PER * 8;
    rst <= '0';

    -- run for >= 10 us (500 cycles @ 20 ns), watching for CPU bus activity.
    -- The reset vector (0x0) is DEV_SRAM (bootram_infer, internal to
    -- cpus_xip.vhd), so instruction fetches aren't visible at the entity
    -- boundary; but cpu0_data_master_en is a real driven output (cpu_core's
    -- data_master_en <= data_master_o.en, the pre-decode data-master strobe)
    -- and cpu0_periph_dbus_o.en toggles on any peripheral access -- either
    -- pulsing proves the CPU came out of reset and is executing boot code,
    -- rather than being wedged. This makes the pass genuinely conditional.
    for i in 0 to 499 loop
      wait until rising_edge(clk);
      if cpu0_data_master_en = '1' or cpu0_periph_dbus_o.en = '1' then
        fetch_seen <= true;
      end if;
    end loop;

    report "cpus_xip_tb: 10us reset+run complete, no bus error; CPU bus activity seen=" & boolean'image(fetch_seen) severity note;

    assert fetch_seen
      report "no CPU bus activity observed in 10us (CPU wedged / didn't leave reset)"
      severity failure;

    report "Test Passed cpus_xip_tb" severity note;
    done <= true;
    wait;
  end process;

end architecture;
