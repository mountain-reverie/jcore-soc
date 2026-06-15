-- M0 single-CPU cpus architecture: identical to targets/cpus_one.vhd's
-- one_cpu except the boot memory binds bootram_infer (inferred ECP5 EBR)
-- instead of the Xilinx memory_fpga.
architecture one_cpu_m0 of cpus is
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

  cpu0_periph_dbus_o <= data_bus_o(DEV_PERIPH);
  data_bus_i(DEV_PERIPH) <= cpu0_periph_dbus_i;
  cpu0_ddr_ibus_o <= instr_bus_o(DEV_DDR);
  instr_bus_i(DEV_DDR) <= cpu0_ddr_ibus_i;
  cpu0_ddr_dbus_o <= data_bus_o(DEV_DDR);
  data_bus_i(DEV_DDR) <= cpu0_ddr_dbus_i;

  cpu1_periph_dbus_o <= NULL_DATA_O;
  cpu1_ddr_ibus_o <= NULL_INST_O;
  cpu1_ddr_dbus_o <= NULL_DATA_O;
  cpu1_mem_lock <= '0';
  cpu1_event_o <= (lvl => (others => '0'), others => '0');
  cpu1_data_master_en <= '0';
  cpu1_data_master_ack <= '0';

  sram : entity work.bootram_infer(inferred)
    generic map (c_addr_width => 14)
    port map (clk => clk, ibus_i => sraminst_o, ibus_o => sraminst_i,
              db_i => sramdt_o, db_o => sramdt_i);

  bootmem_onewait_data : entity work.data_bus_delay (rtl)
      generic map (INSERT_WRITE_DELAY => INSERT_WRITE_DELAY_BOOT_MEM,
                   INSERT_READ_DELAY  => INSERT_READ_DELAY_BOOT_MEM)
      port map (clk => clk, rst => rst,
        master_o => data_bus_o(DEV_SRAM), master_i => data_bus_i(DEV_SRAM),
        slave_o => sramdt_o, slave_i => sramdt_i);

  bootmem_onewait_inst : entity work.instr_bus_delay (rtl)
      generic map (INSERT_DELAY => INSERT_INST_DELAY_BOOT_MEM)
      port map (clk => clk, rst => rst,
        master_o => instr_bus_o(DEV_SRAM), master_i => instr_bus_i(DEV_SRAM),
        slave_o => sraminst_o, slave_i => sraminst_i);

  data_bus_i(DEV_CPU) <= loopback_bus(data_bus_o(DEV_CPU));
end architecture;

-- Bind u_cpu via the cpu repo's cpu_synth_direct configuration (synth/
-- cpu_synth_config.vhd): it binds u_mult => mult(stru) + direct decode +
-- register_file(two_bank). The committed FPGA decode configs omit u_mult,
-- which would leave the multiplier an unbound black box; cpu_synth_direct is
-- the CI-proven ECP5 binding, so we reuse it rather than re-specify it.
configuration one_cpu_m0_direct_fpga of cpus is
  for one_cpu_m0
    for all : cpu_core
      use entity work.cpu_core(arch);
      for arch
        for u_cpu : cpu
          use configuration work.cpu_synth_direct;
        end for;
      end for;
    end for;
  end for;
end configuration;
