library ieee;
use ieee.std_logic_1164.all;
use work.cpu2j0_pack.all;
use work.data_bus_pack.all;
use work.cpu_core_pack.all;

-- iCESugar flash-boot single-CPU cpus architecture. A copy of one_cpu_ebr
-- (targets/boards/icesugar/cpus_one_ebr.vhd) that, instead of running real
-- startup code directly out of the boot EBR, powers on by streaming a
-- payload from the UP5K config SPI flash into the SPRAM main RAM
-- (dev_ddr_spram_boot, muxed by flash_boot_reader while boot_active='1'),
-- holding core0 in reset for the duration, then releases the CPU to boot
-- from SPRAM at 0x10000000 (see boot_image_coremark_pkg.vhd's vector table,
-- loaded into the boot EBR via bootram_infer_coremark).
--
-- ice_spi_io (the real iCE40 SB_IO pad wrapper for the shared config-SPI
-- pins) is NOT instantiated here: its pad side is `inout`, bound to unbound
-- SB_IO primitives that only elaborate under a synthesis flow (or a full
-- inout SB_IO sim model, which does not exist in this tree -- see
-- targets/boards/icesugar/tb/flash_boot_tb.vhd's note on the same issue for
-- flash_boot_reader's unit test). flash_boot_reader's d_cs_n/d_sck/d_mosi/
-- d_miso are logically identical to ice_spi_io's d_* side, so this arch
-- wires the cpus entity's fl_* pins directly to the reader's d_* signals.
-- This is logically equivalent for simulation; Task 8/12's soc_gen synth
-- path is expected to insert the real ice_spi_io SB_IO wrapper between fl_*
-- and the physical pads.
architecture cpus_coremark of cpus is
  constant PAYLOAD_WORDS : natural := 8192;

  signal instr_bus_o : instr_bus_o_t;
  signal instr_bus_i : instr_bus_i_t;
  signal data_bus_o : data_bus_o_t;
  signal data_bus_i : data_bus_i_t;
  signal sraminst_o : cpu_instruction_o_t;
  signal sraminst_i : cpu_instruction_i_t;
  signal sramdt_o : cpu_data_o_t;
  signal sramdt_i : cpu_data_i_t;

  -- flash_boot_reader command/status
  signal boot_start : std_logic;
  signal boot_busy  : std_logic;
  signal boot_done  : std_logic;
  signal boot_active_r : std_logic := '0';   -- '1' from start until done

  -- flash_boot_reader's SPRAM write port -> dev_ddr_spram_boot's boot_* port
  signal fbr_sp_en : std_logic;
  signal fbr_sp_we : std_logic_vector(3 downto 0);
  signal fbr_sp_a  : std_logic_vector(16 downto 2);
  signal fbr_sp_dw : std_logic_vector(31 downto 0);

  -- flash_boot_reader's logic-level SPI signals (== ice_spi_io's d_* side)
  signal fbr_d_cs_n : std_logic;
  signal fbr_d_sck  : std_logic;
  signal fbr_d_mosi : std_logic;

  -- power-on one-shot: pulse boot_start exactly once, on the first clk with
  -- rst='0' after this arch is released from its own reset.
  signal started : std_logic := '0';

  -- CPU reset held until the flash-boot load completes.
  signal core0_rst : std_logic;
begin
  ----------------------------------------------------------------------------
  -- Power-on one-shot start pulse + CPU reset-hold + boot_active latch
  ----------------------------------------------------------------------------
  process (clk) is begin
    if rising_edge(clk) then
      if rst = '1' then
        started <= '0';
        boot_active_r <= '0';
      else
        if started = '0' then
          started <= '1';
        end if;
        if boot_start = '1' then
          boot_active_r <= '1';
        elsif boot_done = '1' then
          boot_active_r <= '0';
        end if;
      end if;
    end if;
  end process;

  -- single-cycle start pulse: fires the cycle "started" first becomes '1'
  boot_start <= '1' when (rst = '0' and started = '0') else '0';

  -- Hold core0 in reset until the flash-boot load has completed.
  core0_rst <= rst or not boot_done;

  -- label is core0 (not cpu0) to avoid clashing with the synopsys group "cpu0"
  -- declared in the cpus entity, which ghdl does not skip.
  core0 : cpu_core
    generic map ( COPRO_DECODE => false )
    port map (
      clk => clk, rst => core0_rst,
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

  ----------------------------------------------------------------------------
  -- flash_boot_reader: streams the payload from config flash into SPRAM.
  ----------------------------------------------------------------------------
  boot_reader : entity work.flash_boot_reader
    generic map (
      FLASH_BASE    => x"100000",
      PAYLOAD_WORDS => PAYLOAD_WORDS)
    port map (
      clk => clk, rst => rst,
      start => boot_start, busy => boot_busy, done => boot_done,
      sp_en => fbr_sp_en, sp_we => fbr_sp_we, sp_a => fbr_sp_a, sp_dw => fbr_sp_dw,
      d_cs_n => fbr_d_cs_n, d_sck => fbr_d_sck, d_mosi => fbr_d_mosi, d_miso => fl_miso);

  -- fl_* pins driven directly from the reader's logic-level SPI signals; see
  -- the architecture header comment for why ice_spi_io is not instantiated
  -- here.
  fl_cs_n <= fbr_d_cs_n;
  fl_sck  <= fbr_d_sck;
  fl_mosi <= fbr_d_mosi;

  -- iCE40 UP5K SPRAM (128 KB) serves the DEV_DDR region as main RAM. Single
  -- port -> dev_ddr_spram_boot arbitrates the instruction and data masters,
  -- with the flash_boot_reader's boot_* port taking exclusive ownership
  -- while boot_active_r='1' (during which core0 is held in reset, so it
  -- issues no requests).
  ddr_spram : entity work.dev_ddr_spram_boot
    port map (clk => clk,
              ibus_i => instr_bus_o(DEV_DDR), ibus_o => instr_bus_i(DEV_DDR),
              dbus_i => data_bus_o(DEV_DDR),  dbus_o => data_bus_i(DEV_DDR),
              boot_active => boot_active_r,
              boot_en => fbr_sp_en, boot_we => fbr_sp_we,
              boot_a => fbr_sp_a, boot_dw => fbr_sp_dw);

  -- Single-core board: tie off all cpu1_* outputs.
  cpu1_periph_dbus_o <= NULL_DATA_O;
  cpu1_ddr_ibus_o <= NULL_INST_O;
  cpu1_ddr_dbus_o <= NULL_DATA_O;
  cpu1_mem_lock <= '0';
  cpu1_event_o <= (lvl => (others => '0'), others => '0');
  cpu1_data_master_en <= '0';
  cpu1_data_master_ack <= '0';

  -- On-chip boot RAM (inferred EBR) serves both instruction and data fetches
  -- for the DEV_SRAM region: here it holds ONLY the coremark vector table
  -- (bootram_infer_coremark / boot_image_coremark_pkg), pointing the SH-2
  -- reset PC/SP at the SPRAM payload loaded by flash_boot_reader.
  -- bootram_infer is 0-wait (falling-edge read), so no data_bus_delay /
  -- instr_bus_delay wrappers are needed.
  sram : entity work.bootram_infer_coremark(inferred)
    generic map (c_addr_width => 11)
    port map (clk => clk, ibus_i => sraminst_o, ibus_o => sraminst_i,
              db_i => sramdt_o, db_o => sramdt_i);

  sramdt_o <= data_bus_o(DEV_SRAM);
  data_bus_i(DEV_SRAM) <= sramdt_i;
  sraminst_o <= instr_bus_o(DEV_SRAM);
  instr_bus_i(DEV_SRAM) <= sraminst_i;

  data_bus_i(DEV_CPU) <= loopback_bus(data_bus_o(DEV_CPU));
end architecture;
