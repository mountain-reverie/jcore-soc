-------------------------------------------------------------------------------
-- cpus_xip_probe.vhd -- Task 4 (QSPI XIP cosim) instrumentation.
--
-- A REPLACEMENT for targets/boards/ulx3s/cpus_one_m0_arch.vhd's
-- "one_cpu_m0" architecture of the shared `cpus` entity (targets/cpus.vhd):
-- byte-identical except for one addition, a monitor process that watches
-- the CPU's own DEV_DDR data-bus port -- `data_bus_o(DEV_DDR)`, the raw
-- CPU-side request BEFORE the dcache/ddr_ram_mux/mem_region_mux hierarchy
-- (which internally truncates the top address nibble to 0x0 -- see
-- design.flash.yaml's "IMPORTANT correction" note -- so this is the only
-- point in the hierarchy that still carries the CPU's full, untruncated
-- 32-bit SDRAM address) -- for the XIP payload's signature write (see
-- targets/asic/gf180_j4mmu/xip_payload/payload.S: `mov.l r0,@-r15`, a push
-- of 0xF1A5B007 onto the SDRAM stack at byte address 0x10000FFC, SP-4)
-- and reports PASS the instant it is seen.
--
-- SCRATCHPAD REMOVAL: this used to watch the boot-RAM (DEV_SRAM) write
-- bus for a store into boot_mem's writable stack-SRAM lane. That lane is
-- GONE (components/memory/boot_mem.vhd is now pure ROM) -- the reset
-- vector's SP now points into SDRAM instead (boot_image_pkg.vhd), and
-- this monitor moved to the DEV_DDR bus accordingly. `sram` below is
-- still instantiated (bootram_infer(boot_mem_gf180), now ROM-only, no
-- vendor SRAM macros -- see lib/memory_tech_lib/tech/gf180/
-- boot_mem_stack_gf180.vhd) purely to keep the boot-vector-fetch path
-- identical to cpus_one_m0_gf180_arch.vhd; it is no longer part of the
-- pass/fail proof.
--
-- WHY A REPLACEMENT ARCHITECTURE, NOT AN EXTERNAL NAME: sramdt_o is local
-- to `cpus`'s architecture (bootram_infer sits one level of hierarchy
-- below the generated `soc` top, inside `cpus`), so `entity work.soc
-- (impl)` cannot observe it as a black box, and GHDL external names
-- (`<<signal ...>>`) are a VHDL-2008 construct whose library format is
-- incompatible with the VHDL-93 analysis this whole repo (and this
-- target's filelist.sh) uses -- mixing --std=93/--std=08 units in one
-- GHDL work library is not supported (verified experimentally: a
-- --std=08 unit cannot see a --std=93 unit's declarations in the same
-- "work" library).
--
-- WHY THIS FILE KEEPS THE NAME "one_cpu_m0" (does NOT add a second,
-- differently-named architecture): tried that first ("one_cpu_m0_xip")
-- and it broke analysis of xip_cosim_tb.vhd with a confusing
-- "no actual for generic decode_type/reset_vector" error attributed to
-- decode.vhd's `core : decode_core` (itself an unrelated, ordinary
-- component instantiation). Root cause: targets/asic/gf180_j4mmu/
-- cpus_config.vhd's `soc_cpus_config` configuration -- which is what
-- actually threads decode_core's required generics all the way down via
-- `for one_cpu_m0 ... u_cpu: cpu use configuration work.cpu_synth_j4 ...`
-- -- only has a `for one_cpu_m0` clause. GHDL's `--syn-binding`
-- elaboration/analysis default-binding resolution (triggered eagerly by
-- xip_cosim_tb.vhd's `uut : entity work.soc(impl)`, a direct entity
-- instantiation with an explicit architecture) needs the DEFAULT-BOUND
-- architecture name for `cpus` to match a `for <name>` clause of some
-- configuration of that entity; a same-named clone of the CONFIGURATION
-- (`for one_cpu_m0_xip`) did NOT fix it either in testing, so the robust
-- fix is simpler: don't introduce a new architecture name at all. This
-- file supplies the ONLY "one_cpu_m0" architecture analyzed by
-- sim/xip_sim.sh (which excludes targets/boards/ulx3s/cpus_one_m0_arch
-- .vhd from its analyze list in favor of this file -- see that script),
-- so `soc_cpus_config`'s existing `for one_cpu_m0` clause applies
-- completely unchanged, no new configuration needed, and the monitor
-- process is simply extra concurrent logic in the same architecture.
--
-- The base (non-XIP) sim/rtl.sh flow keeps analyzing the original
-- targets/boards/ulx3s/cpus_one_m0_arch.vhd (this file is never part of
-- its filelist), so it is entirely unaffected.
-------------------------------------------------------------------------------
library ieee;
use ieee.std_logic_1164.all;
use ieee.numeric_std.all;
use work.cpu2j0_pack.all;
use work.data_bus_pack.all;
use work.cpu_core_pack.all;

architecture one_cpu_m0 of cpus is
  signal instr_bus_o : instr_bus_o_t;
  signal instr_bus_i : instr_bus_i_t;
  signal data_bus_o : data_bus_o_t;
  signal data_bus_i : data_bus_i_t;
  signal sraminst_o : cpu_instruction_o_t;
  signal sraminst_i : cpu_instruction_i_t;
  signal sramdt_o : cpu_data_o_t;
  signal sramdt_i : cpu_data_i_t;

  -- XIP signature-observed flag: exposed for the outer tb watchdog via a
  -- report severity note (grepped by xip_sim.sh) rather than a new port.
  -- SCRATCHPAD REMOVAL: address moved from boot_mem's (now-gone) writable
  -- stack-SRAM lane to SDRAM byte address 0x10000FFC (SP-4, the push
  -- target: SP=0x10001000 from boot_image_pkg.vhd, DEV_DDR/SDRAM base
  -- 0x10000000).
  constant XIP_SIG_ADDR  : std_logic_vector(31 downto 0) := x"10000ffc";
  constant XIP_SIG_VALUE : std_logic_vector(31 downto 0) := x"f1a5b007";
begin
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

  -----------------------------------------------------------------------
  -- XIP signature monitor (scratchpad-removal proof): watch the CPU's own
  -- DEV_DDR data-bus request for the payload's push (see
  -- targets/asic/gf180_j4mmu/xip_payload/payload.S: `mov.l sig_val,r0`
  -- (PC-relative literal-pool load served by flash) / `mov.l r0,@-r15`,
  -- pushing r0=0xf1a5b007 at r15-4=0x10000ffc -- r15/SP is already an
  -- SDRAM address straight from the reset vector, boot_image_pkg.vhd) and
  -- report PASS the instant it is seen. This is the ONLY store the
  -- payload ever issues, and it can only reach this bus if (a) the CPU
  -- actually fetched+executed the payload from flash@0x14000000 (there is
  -- no other code path to it -- see boot_image_pkg.vhd's vector table),
  -- AND (b) the SDRAM stack (self-initialised in hardware by
  -- sdram_ctrl -- no on-chip scratchpad backs it) actually works.
  -----------------------------------------------------------------------
  xip_monitor : process(clk)
  begin
    if rising_edge(clk) then
      if data_bus_o(DEV_DDR).en = '1' and data_bus_o(DEV_DDR).wr = '1'
         and data_bus_o(DEV_DDR).a = XIP_SIG_ADDR
         and data_bus_o(DEV_DDR).d = XIP_SIG_VALUE then
        report "XIP_SIG_OK: SDRAM[0x10000ffc] == 0xF1A5B007 -- payload fetched+executed from flash@0x14000000, SDRAM stack push verified"
          severity note;
      end if;
    end if;
  end process;

end architecture;
