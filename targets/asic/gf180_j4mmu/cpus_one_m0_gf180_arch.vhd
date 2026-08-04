library ieee;
use ieee.std_logic_1164.all;
use work.cpu2j0_pack.all;
use work.data_bus_pack.all;
use work.cpu_core_pack.all;

-- gf180_j4mmu-only cpus architecture: identical to
-- targets/boards/ulx3s/cpus_one_m0_arch.vhd's one_cpu_m0 (BM Task 2) except
-- the boot memory binds bootram_infer(boot_mem_gf180) (c_addr_width=>12, see
-- lib/memory_tech_lib/tech/gf180/boot_mem_stack_gf180.vhd) instead of
-- (boot_mem)'s tech/inferred array-of-flops -- functionally equivalent by
-- construction (see that file's header). NOTE (SCRATCHPAD REMOVAL): boot_mem_gf180
-- is now a PURE READ-ONLY constant ROM (the boot vector table); it has no vendor
-- SRAM macro. The earlier writable-stack lane (0x800-0xFFF, once backed by 4x
-- gf180mcu_fd_ip_sram__sram512x8m8wm1 macros) is GONE -- sdram_ctrl self-initialises
-- in hardware so the reset vector's SP points into SDRAM (see boot_image_pkg.vhd).
-- The base variant
-- (cpus_one_m0_arch.vhd, shared with ulx3s/icesugar) is UNCHANGED and keeps
-- bootram_infer(inferred)'s single 16 KiB read/write EBR array
-- (c_addr_width=>14) -- this rebind is flash-variant-only.
--
-- Rationale for a separate arch rather than a configuration-level override:
-- `sram : entity work.bootram_infer(inferred)` in cpus_one_m0_arch.vhd is a
-- direct entity instantiation (entity+architecture named directly on the
-- instantiation label), not a component instantiation -- VHDL configuration
-- declarations/specifications only rebind component instantiations, so a
-- gf180-side configuration cannot retarget this binding without editing the
-- shared ulx3s file. This file duplicates the (small) cpus_one_m0
-- architecture body verbatim except for the `sram` binding/generic, so
-- ulx3s/icesugar/other boards keep using cpus_one_m0_arch.vhd's
-- bootram_infer(inferred)/16 KiB unchanged.
-- Task 6 PHASE C: cpus_config.vhd's soc_cpus_config configuration (which
-- threads u_cpu's required decode_type/reset_vector generics down via
-- `for one_cpu_m0 ... u_cpu: cpu use configuration work.cpu_synth_j4
-- generic map (PRIV_ARCH => true)`) only has a
-- `for one_cpu_m0` clause -- it does not, and structurally cannot without
-- editing the shared base file, apply to this architecture's different
-- name (`one_cpu_m0_gf180`). Every existing flow that successfully
-- elaborates this architecture (sim/xip_sim.sh) works around that by
-- substituting a differently-purposed file (tb/cpus_xip_probe.vhd) that
-- happens to reuse the name "one_cpu_m0" so soc_cpus_config's clause
-- matches it instead -- this architecture itself was never actually
-- elaborated standalone before Task 6 PHASE C's top-harden dry run
-- surfaced the gap ("no actual for generic decode_type/reset_vector" on
-- decode.vhd's `core : decode_core`, GHDL default component-binding does
-- NOT auto-select a configuration for a plain component instantiation like
-- `u_cpu : cpu` two levels down inside cpu_core's single architecture).
-- Fix: an equivalent self-contained configuration, named for this
-- architecture, so this file has no dependency on any external soc/cpus
-- level configuration to elaborate correctly on its own.
configuration cpu_core_j4mmu_gf180 of cpu_core is
  for arch
    for u_cpu : cpu
      use configuration work.cpu_synth_j4
        generic map (
          PRIV_ARCH => true
        );
    end for;
  end for;
end configuration;

architecture one_cpu_m0_gf180 of cpus is
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
  core0 : configuration work.cpu_core_j4mmu_gf180
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

  -- boot_mem (not inferred), c_addr_width=>12 (4 KiB window): a pure read-only
  -- constant ROM (boot vector table), no vendor SRAM -- the boot-time stack lives
  -- in SDRAM (see boot_image_pkg.vhd), not an on-chip scratchpad.
  sram : entity work.bootram_infer(boot_mem_gf180)
    generic map (c_addr_width => 12)
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
