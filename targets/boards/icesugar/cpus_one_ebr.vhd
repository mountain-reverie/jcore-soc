library ieee;
use ieee.std_logic_1164.all;
use work.cpu2j0_pack.all;
use work.data_bus_pack.all;
use work.cpu_core_pack.all;

-- iCESugar EBR-only single-CPU cpus architecture. Derived from the ULX3S
-- one_cpu_m0 arch (targets/boards/ulx3s/cpus_one_m0_arch.vhd) but with NO
-- SDRAM: ALL memory (instruction + data) is served from bootram_infer (inferred
-- iCE40 EBR). The SuperH reset vector is 0x00000000, which decode_core_*_addr
-- maps to DEV_SRAM, so the boot RAM serves the reset fetch and all subsequent
-- code/data. The DEV_DDR slots of the core buses are served by an iCE40 UP5K
-- SPRAM (128 KB) main-RAM instance (dev_ddr_spram); the cpus entity's
-- external DDR ports are still driven NULL (SPRAM is internal to this arch).
-- The single J1 core has no coprocessor (COPRO_DECODE => false). All cpu1_*
-- outputs are tied off (single-core board).
architecture one_cpu_ebr of cpus is
  signal instr_bus_o : instr_bus_o_t;
  signal instr_bus_i : instr_bus_i_t;
  signal data_bus_o : data_bus_o_t;
  signal data_bus_i : data_bus_i_t;
  signal sraminst_o : cpu_instruction_o_t;
  signal sraminst_i : cpu_instruction_i_t;
  signal sramdt_o : cpu_data_o_t;
  signal sramdt_i : cpu_data_i_t;
begin
  -- label is core0 (not cpu0) to avoid clashing with the synopsys group "cpu0"
  -- declared in the cpus entity, which ghdl does not skip.
  core0 : cpu_core
    generic map ( COPRO_DECODE => false )
    port map (
      clk => clk, rst => rst,
      instr_bus_o => instr_bus_o, instr_bus_i => instr_bus_i,
      data_bus_lock => cpu0_mem_lock,
      data_bus_o => data_bus_o, data_bus_i => data_bus_i,
      debug_o => debug_o, debug_i => debug_i,
      event_o => cpu0_event_o, event_i => cpu0_event_i,
      data_master_en => cpu0_data_master_en, data_master_ack => cpu0_data_master_ack,
      copro_i => cpu0_copro_i, copro_o => cpu0_copro_o);

  -- Peripheral bus (DEV_PERIPH) out to the generated SoC.
  cpu0_periph_dbus_o <= data_bus_o(DEV_PERIPH);
  data_bus_i(DEV_PERIPH) <= cpu0_periph_dbus_i;

  -- No external DDR/SDRAM on iCESugar: tie the cpus entity's DDR ports to
  -- NULL (nothing leaves this arch on those ports).
  cpu0_ddr_ibus_o <= NULL_INST_O;
  cpu0_ddr_dbus_o <= NULL_DATA_O;

  -- iCE40 UP5K SPRAM (128 KB) serves the DEV_DDR region as main RAM. Single
  -- port -> dev_ddr_spram arbitrates the instruction and data masters.
  ddr_spram : entity work.dev_ddr_spram
    port map (clk => clk,
              ibus_i => instr_bus_o(DEV_DDR), ibus_o => instr_bus_i(DEV_DDR),
              dbus_i => data_bus_o(DEV_DDR),  dbus_o => data_bus_i(DEV_DDR));

  -- Single-core board: tie off all cpu1_* outputs.
  cpu1_periph_dbus_o <= NULL_DATA_O;
  cpu1_ddr_ibus_o <= NULL_INST_O;
  cpu1_ddr_dbus_o <= NULL_DATA_O;
  cpu1_mem_lock <= '0';
  cpu1_event_o <= (lvl => (others => '0'), others => '0');
  cpu1_data_master_en <= '0';
  cpu1_data_master_ack <= '0';

  -- On-chip boot RAM (inferred EBR) serves both instruction and data fetches
  -- for the DEV_SRAM region. bootram_infer is 0-wait (falling-edge read), so no
  -- data_bus_delay / instr_bus_delay wrappers are needed.
  sram : entity work.bootram_infer(inferred)
    generic map (c_addr_width => 11)
    port map (clk => clk, ibus_i => sraminst_o, ibus_o => sraminst_i,
              db_i => sramdt_o, db_o => sramdt_i);

  sramdt_o <= data_bus_o(DEV_SRAM);
  data_bus_i(DEV_SRAM) <= sramdt_i;
  sraminst_o <= instr_bus_o(DEV_SRAM);
  instr_bus_i(DEV_SRAM) <= sraminst_i;

  data_bus_i(DEV_CPU) <= loopback_bus(data_bus_o(DEV_CPU));
end architecture;
