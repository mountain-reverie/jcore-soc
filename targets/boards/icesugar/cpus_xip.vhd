library ieee;
use ieee.std_logic_1164.all;
use work.cpu2j0_pack.all;
use work.data_bus_pack.all;
use work.cpu_core_pack.all;
use work.spi_page_cache_pack.all;

-- iCESugar single-CPU XIP arch (Sub-project B, Task 5). Derived from
-- cpus_one_ebr.vhd (EBR-only single-CPU, no external DDR/SDRAM): all boot
-- code/data still comes from bootram_infer (DEV_SRAM) and the 128 KB UP5K
-- SPRAM (dev_ddr_spram, DEV_DDR) is still the main RAM. On top of that, this
-- arch adds a demand-paged XIP flash window: the window/frames live in the
-- 0x1/DEV_DDR nibble ABOVE the SPRAM (window a(31:20)=x"108", frames
-- a(31:20)=x"109"; SPRAM occupies a(31:20)=x"100"-x"100"+..., i.e.
-- 0x10000000-0x1001FFFF), so the CPU's existing decode_core_*_addr routing
-- of nibble 0x1 -> DEV_DDR (targets/cpu_core_pkg.vhd, unmodified) already
-- carries window/frame fetches out the DEV_DDR instr/data bus ports -- no
-- cpu_core/cpu_core_pkg change needed.
--
-- The core new wiring vs. cpus_one_ebr is therefore a DEV_DDR sub-decode:
-- the spi_page_cache instance snoops the CPU's pre-mux master buses
-- (instr_master_snoop/data_master_snoop) to detect window addresses and
-- serve transparent hits (instr_win_i/data_win_i) or raise a sideband
-- page_fault_o (wired to cpu_core's page_fault_i, PAGE_FAULT_ARCH => true).
-- The DEV_DDR return is then muxed per cycle: window/frame hits come from
-- the page cache, everything else (the SPRAM range) comes from
-- dev_ddr_spram, exactly as cpus_one_ebr wires it.
--
-- The page cache's MMIO register slave (reg_i/reg_o, PC_MMIO_BASE =
-- 0xABCD0400) is sub-decoded off the DEV_PERIPH bus the same way (address
-- match gates reg_i.en via mask_data_o; devices.vhd, which is soc_gen
-- generated from design.yaml and not touched by this task, does not yet know
-- about this device -- any peripheral access it does not claim falls through
-- to the external cpu0_periph_dbus_i port unaffected).
--
-- Flash pins: the generated `cpus` entity (targets/cpus.vhd) exposes no pin
-- ports at all (only bus/event/copro ports), so ice_spi_io's pin_* side is
-- left on internal, otherwise-unconnected signals here -- there is no board
-- pad to drive them to yet. Wiring those to real iCESugar config-flash pads
-- is Task 8's padring/pins.icesugar concern; d_* (the digital side) is fully
-- wired to spi_page_cache here.
--
-- Single J1 core, no coprocessor (COPRO_DECODE => false); all cpu1_*
-- outputs tied off (single-core board), same as cpus_one_ebr.
-- Binds cpu_core(arch)'s embedded `u_cpu : cpu` component instantiation
-- (unconfigured -- decode_core's decode_type/reset_vector generics need an
-- explicit variant selection) to the same iCESugar J1-DSP synth variant
-- cpus_config.vhd's soc_cpus_config uses for cpus_one_ebr's core0. Declared
-- here (rather than relying on an external top-level configuration, as
-- cpus_one_ebr/cpus_config.vhd do) because core0 below is a direct entity
-- instantiation (needed for cpu_core(arch)'s new Task 4 ports/generics, which
-- the stale `cpu_core` component in cpu_core_pkg.vhd doesn't expose) --
-- direct entity instantiations can only be bound via a configuration
-- instantiation of themselves, not an enclosing top-level configuration.
configuration one_cpu_xip_core_cfg of cpu_core is
  for arch
    for u_cpu : cpu
      use configuration work.cpu_synth_j1_dsp;
    end for;
  end for;
end configuration;

-- Context clauses apply only to the immediately following design unit, so
-- the architecture below (a separate design unit from the configuration
-- above) needs its own copy of the use clauses at the top of this file.
library ieee;
use ieee.std_logic_1164.all;
use work.cpu2j0_pack.all;
use work.data_bus_pack.all;
use work.cpu_core_pack.all;
use work.spi_page_cache_pack.all;

architecture one_cpu_xip of cpus is
  signal instr_bus_o : instr_bus_o_t;
  signal instr_bus_i : instr_bus_i_t;
  signal data_bus_o : data_bus_o_t;
  signal data_bus_i : data_bus_i_t;
  signal sraminst_o : cpu_instruction_o_t;
  signal sraminst_i : cpu_instruction_i_t;
  signal sramdt_o : cpu_data_o_t;
  signal sramdt_i : cpu_data_i_t;

  -- DEV_DDR: dev_ddr_spram's masked-input request and its return.
  -- dev_ddr_spram only decodes a(16:2) (it ignores a(31:20)), so window/frame
  -- accesses (a(31:20) in {x"108",x"109"}) MUST be masked off its inputs or
  -- they alias into real SPRAM at the low offset -- a store would corrupt
  -- SPRAM and a read would needlessly burn the port. spram_*bus_o forces .en
  -- (and hence .rd/.wr/.we) low on a window/frame hit.
  signal spram_ibus_o : cpu_instruction_o_t;
  signal spram_dbus_o : cpu_data_o_t;
  signal spram_ibus_i : cpu_instruction_i_t;
  signal spram_dbus_i : cpu_data_i_t;

  -- CPU pre-mux master snoop taps (Task 4 ports), observed by the page cache
  signal instr_master_snoop : cpu_instruction_o_t;
  signal data_master_snoop  : cpu_data_o_t;

  -- spi_page_cache window/fault/MMIO/flash-pin signals
  signal pc_instr_win_i : cpu_instruction_i_t;
  signal pc_data_win_i  : cpu_data_i_t;
  signal pc_win_instr_sel : std_logic;
  signal pc_win_data_sel  : std_logic;
  signal pc_fault : cpu_page_fault_i_t;
  signal pc_reg_i : cpu_data_i_t;
  signal pc_reg_o : cpu_data_o_t;
  signal pc_mmio_sel : std_logic;
  signal pc_d_cs_n  : std_logic;
  signal pc_d_sck   : std_logic;
  signal pc_d_mosi  : std_logic;
  signal pc_d_miso  : std_logic;

  -- ice_spi_io pad-side signals: no cpus-entity pin ports exist yet (see
  -- header note), so these are left internal/unconnected (Task 8).
  signal pin_cs_n  : std_logic;
  signal pin_sck   : std_logic;
  signal pin_mosi  : std_logic;
  signal pin_miso  : std_logic := '0';
begin
  -- label is core0 (not cpu0) to avoid clashing with the synopsys group "cpu0"
  -- declared in the cpus entity, which ghdl does not skip.
  -- Direct entity instantiation (not the `cpu_core` component declared in
  -- cpu_core_pkg.vhd, which predates Task 4's PAGE_FAULT_ARCH generic and
  -- page_fault_i/instr_master_snoop/data_master_snoop ports and so doesn't
  -- expose them) so this arch can use the new Task 4 surface without
  -- modifying the shared, read-only cpu_core_pkg.vhd.
  core0 : configuration work.one_cpu_xip_core_cfg
    generic map ( COPRO_DECODE => false, PAGE_FAULT_ARCH => true )
    port map (
      clk => clk, rst => rst,
      instr_bus_o => instr_bus_o, instr_bus_i => instr_bus_i,
      data_bus_lock => cpu0_mem_lock,
      data_bus_o => data_bus_o, data_bus_i => data_bus_i,
      debug_o => debug_o, debug_i => debug_i,
      event_o => cpu0_event_o, event_i => cpu0_event_i,
      data_master_en => cpu0_data_master_en, data_master_ack => cpu0_data_master_ack,
      copro_i => cpu0_copro_i, copro_o => cpu0_copro_o,
      page_fault_i => pc_fault,
      instr_master_snoop => instr_master_snoop,
      data_master_snoop => data_master_snoop);

  -- Peripheral bus (DEV_PERIPH) out to the generated SoC, sub-decoded for the
  -- page cache's MMIO register slave (PC_MMIO_BASE = 0xABCD0400). Any access
  -- devices.vhd doesn't recognize (including this one, until Task 8 teaches
  -- design.yaml about it) simply falls through as a NONE/loopback there; here
  -- we steer the reply back from whichever side actually claims the address.
  pc_mmio_sel <= '1' when data_bus_o(DEV_PERIPH).a(31 downto 5) = PC_MMIO_BASE(31 downto 5) else '0';
  pc_reg_o <= mask_data_o(data_bus_o(DEV_PERIPH), pc_mmio_sel);

  cpu0_periph_dbus_o <= data_bus_o(DEV_PERIPH);
  data_bus_i(DEV_PERIPH) <= pc_reg_i when pc_mmio_sel = '1' else cpu0_periph_dbus_i;

  -- No external DDR/SDRAM on iCESugar: tie the cpus entity's DDR ports to
  -- NULL (nothing leaves this arch on those ports).
  cpu0_ddr_ibus_o <= NULL_INST_O;
  cpu0_ddr_dbus_o <= NULL_DATA_O;

  -- iCE40 UP5K SPRAM (128 KB) serves the DEV_DDR region as main RAM. Single
  -- port -> dev_ddr_spram arbitrates the instruction and data masters. Its
  -- reply (spram_ibus_i/spram_dbus_i) is the DEV_DDR default; the window mux
  -- below overrides it with the page cache's reply for window/frame hits.
  --
  -- Mask the SPRAM's INPUTS so only true SPRAM-range accesses (a(31:20)=x"100")
  -- reach it: a window/frame hit forces .en low (mask_data_o/mask_instruction_o
  -- also zero .rd/.wr/.we accordingly) so the window/frame address never
  -- aliases into SPRAM (dev_ddr_spram ignores a(31:20), decoding only a(16:2)).
  spram_ibus_o <= mask_instruction_o(instr_bus_o(DEV_DDR), not pc_win_instr_sel);
  spram_dbus_o <= mask_data_o(data_bus_o(DEV_DDR), not pc_win_data_sel);

  ddr_spram : entity work.dev_ddr_spram
    port map (clk => clk,
              ibus_i => spram_ibus_o, ibus_o => spram_ibus_i,
              dbus_i => spram_dbus_o, dbus_o => spram_dbus_i);

  -- XIP page cache: observes the pre-decode master buses, serves window/
  -- frame hits transparently, raises the sideband page fault on miss, and
  -- exposes its MMIO regs + flash SPI pins.
  page_cache : entity work.spi_page_cache
    port map (
      clk => clk, rst => rst,
      instr_master_o => instr_master_snoop,
      data_master_o  => data_master_snoop,
      instr_win_i => pc_instr_win_i,
      data_win_i  => pc_data_win_i,
      win_instr_sel => pc_win_instr_sel,
      win_data_sel  => pc_win_data_sel,
      page_fault_o => pc_fault,
      reg_i => pc_reg_o,
      reg_o => pc_reg_i,
      d_cs_n => pc_d_cs_n, d_sck => pc_d_sck, d_mosi => pc_d_mosi, d_miso => pc_d_miso);

  -- DEV_DDR sub-decode: the window/frame nibble (a(31:20) in {x"108",x"109"})
  -- is served by the page cache; everything else in the DEV_DDR range (the
  -- SPRAM, a(31:20)=x"100") is served by dev_ddr_spram, same as cpus_one_ebr.
  instr_bus_i(DEV_DDR) <= pc_instr_win_i when pc_win_instr_sel = '1' else spram_ibus_i;
  data_bus_i(DEV_DDR)  <= pc_data_win_i  when pc_win_data_sel  = '1' else spram_dbus_i;

  -- Config-flash SPI pins (digital side wired; pad side is internal-only
  -- until Task 8's padring work exposes real cpus-entity pin ports).
  spi_io : entity work.ice_spi_io
    port map (
      d_cs_n => pc_d_cs_n, d_sck => pc_d_sck, d_mosi => pc_d_mosi, d_miso => pc_d_miso,
      pin_cs_n => pin_cs_n, pin_sck => pin_sck, pin_mosi => pin_mosi, pin_miso => pin_miso);

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
