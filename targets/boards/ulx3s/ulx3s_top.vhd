library ieee;
use ieee.std_logic_1164.all;
use ieee.numeric_std.all;
use work.cpu2j0_pack.all;
use work.data_bus_pack.all;
use work.dma_pack.all;
use work.sdram_pkg.all;
use work.config.all;

entity ulx3s_top is
  port (
    clk_25mhz : in  std_logic;
    ftdi_txd  : out std_logic;   -- FPGA -> host (our TX)
    ftdi_rxd  : in  std_logic;   -- host -> FPGA (our RX)
    btn       : in  std_logic_vector(6 downto 0);
    led       : out std_logic_vector(7 downto 0);
    -- SDRAM (16-bit SDR; the J2's DEV_DDR main memory)
    sdram_clk  : out   std_logic;
    sdram_cke  : out   std_logic;
    sdram_csn  : out   std_logic;
    sdram_rasn : out   std_logic;
    sdram_casn : out   std_logic;
    sdram_wen  : out   std_logic;
    sdram_dqm  : out   std_logic_vector(1 downto 0);
    sdram_a    : out   std_logic_vector(12 downto 0);
    sdram_ba   : out   std_logic_vector(1 downto 0);
    sdram_d    : inout std_logic_vector(15 downto 0));
end entity;

architecture rtl of ulx3s_top is
  signal clk_cpu : std_logic;
  signal pll_locked : std_logic;
  signal rst : std_logic := '1';
  signal rst_sync : std_logic_vector(1 downto 0) := "11";
  signal ext_rst : std_logic;

  signal cpu0_periph_dbus_o : cpu_data_o_t;
  signal cpu0_periph_dbus_i : cpu_data_i_t;
  signal cpu0_ddr_dbus_o : cpu_data_o_t;
  signal cpu0_ddr_dbus_i : cpu_data_i_t;
  signal cpu0_ddr_ibus_o : cpu_instruction_o_t;
  signal cpu0_ddr_ibus_i : cpu_instruction_i_t;

  signal cpu0_mem_lock : std_logic;
  -- DDR subsystem (mux -> sdram_ctrl -> iocells)
  signal ddr_bus_o : cpu_data_o_t;
  signal ddr_bus_i : cpu_data_i_t;
  signal ddr_burst : std_logic;
  signal ddr_bus_ack_r : std_logic;
  signal dma_dbus_o : bus_ddr_o_t;
  signal sd_cmd : sdram_cmd_t;
  signal sd_dq_o, sd_dq_i : std_logic_vector(15 downto 0);
  signal sd_dq_oe : std_logic;

  signal uart_tx : std_logic;

  -- M2: peripheral bus split (periph_mux) + AIC v1 (irq controller + RTC + PIT)
  signal uart_dbus_o, aic_dbus_o, gpio_dbus_o : cpu_data_o_t;
  signal uart_dbus_i, aic_dbus_i, gpio_dbus_i : cpu_data_i_t;
  signal cpu0_event_i_s : cpu_event_i_t;   -- AIC -> CPU
  signal cpu0_event_o_s : cpu_event_o_t;   -- CPU -> AIC
  signal aic_irq : std_logic_vector(7 downto 0);
  signal aic_rtc_sec : std_logic_vector(63 downto 0);
  signal aic_rtc_nsec : std_logic_vector(31 downto 0);
  signal gpio_d_i, gpio_d_o, gpio_d_t : std_logic_vector(31 downto 0);

  -- clkgen as a component: no configuration is used; the sim and synth flows
  -- analyze different clkgen source files so the last-analyzed architecture
  -- (sim for tb, ecp5 for synth) wins default binding.
  component clkgen
    port (clk_in : in std_logic; rst_in : in std_logic;
          clk : out std_logic; locked : out std_logic);
  end component;
begin
  -- ULX3S btn(0) (FIRE1) as external reset: active-high when pressed, idle low
  -- (PULLMODE=DOWN in ulx3s.lpf), so the board leaves reset automatically at
  -- power-on with no button interaction.
  ext_rst <= btn(0);

  clk : clkgen
    port map (clk_in => clk_25mhz, rst_in => ext_rst, clk => clk_cpu, locked => pll_locked);

  -- reset synchronizer: assert while PLL unlocked, release after sync
  process(clk_cpu, pll_locked)
  begin
    if pll_locked = '0' then
      rst_sync <= "11";
    elsif rising_edge(clk_cpu) then
      rst_sync <= rst_sync(0) & '0';
    end if;
  end process;
  rst <= rst_sync(1);

  cpus : configuration work.one_cpu_m0_direct_fpga
    generic map (insert_inst_delay_boot_mem => false,
                 insert_read_delay_boot_mem => false,
                 insert_write_delay_boot_mem => false)
    port map (
      clk => clk_cpu, rst => rst,
      cpu0_copro_i => NULL_COPR_I, cpu0_copro_o => open,
      cpu0_data_master_ack => open, cpu0_data_master_en => open,
      cpu0_ddr_dbus_i => cpu0_ddr_dbus_i, cpu0_ddr_dbus_o => cpu0_ddr_dbus_o,
      cpu0_ddr_ibus_i => cpu0_ddr_ibus_i, cpu0_ddr_ibus_o => cpu0_ddr_ibus_o,
      cpu0_event_i => cpu0_event_i_s, cpu0_event_o => cpu0_event_o_s,
      cpu0_mem_lock => cpu0_mem_lock,
      cpu0_periph_dbus_i => cpu0_periph_dbus_i, cpu0_periph_dbus_o => cpu0_periph_dbus_o,
      cpu1_copro_i => NULL_COPR_I, cpu1_copro_o => open,
      cpu1_data_master_ack => open, cpu1_data_master_en => open,
      cpu1_ddr_dbus_i => (d => (others => '0'), ack => '0'), cpu1_ddr_dbus_o => open,
      cpu1_ddr_ibus_i => (d => (others => '0'), ack => '0'), cpu1_ddr_ibus_o => open,
      cpu1_event_i => NULL_CPU_EVENT_I, cpu1_event_o => open,
      cpu1_mem_lock => open,
      cpu1_periph_dbus_i => (d => (others => '0'), ack => '0'), cpu1_periph_dbus_o => open,
      cpu1eni => '0', debug_i => CPU_DEBUG_NOP, debug_o => open);

  -- DDR subsystem: ddr_ram_mux (icache enabled, dcache bypassed) -> sdram_ctrl
  -- -> sdram_iocells -> SDRAM pins. The J2 reaches DEV_DDR (0x10000000) here.
  dma_dbus_o <= (en => '0', a => (others => '0'), d => (others => '0'),
                wr => '0', we => (others => '0'),
                burst32 => '0', burst16 => '0', bgrp => '0');

  ddr_mux : configuration work.ddr_ram_mux_one_cpu_idcache_fpga
    port map (
      clk => clk_cpu, clk_ddr => clk_cpu, rst => rst,
      cpu0_ibus_o => cpu0_ddr_ibus_o, cpu0_ibus_i => cpu0_ddr_ibus_i,
      cpu0_dbus_o => cpu0_ddr_dbus_o, cpu0_dbus_i => cpu0_ddr_dbus_i,
      cpu0_mem_lock => cpu0_mem_lock,
      cpu1_ibus_o => NULL_INST_O, cpu1_ibus_i => open,
      cpu1_dbus_o => NULL_DATA_O, cpu1_dbus_i => open,
      cpu1_mem_lock => '0',
      dma_dbus_o => dma_dbus_o, dma_dbus_i => open,
      icache0_ctrl => (en => '1', inv => '0'), dcache0_ctrl => (en => '0', inv => '0'),
      icache1_ctrl => (en => '0', inv => '0'), dcache1_ctrl => (en => '0', inv => '0'),
      cache01sel_ctrl_temp => '0',
      ddr_bus_o => ddr_bus_o, ddr_bus_i => ddr_bus_i,
      ddr_burst => ddr_burst, ddr_bus_ack_r => ddr_bus_ack_r);

  sdram : entity work.sdram_ctrl(rtl)
    port map (
      clk => clk_cpu, rst => rst,
      req => ddr_bus_o, bst => ddr_burst, resp => ddr_bus_i, ack_r => ddr_bus_ack_r,
      cmd => sd_cmd, dq_o => sd_dq_o, dq_oe => sd_dq_oe, dq_i => sd_dq_i);

  sd_io : entity work.sdram_iocells(rtl)
    port map (dq_o => sd_dq_o, dq_oe => sd_dq_oe, dq_i => sd_dq_i, dq => sdram_d);

  sdram_clk  <= clk_cpu;          -- in-phase (M1b); ODDR/phase-shift is a hw follow-on
  sdram_cke  <= sd_cmd.cke;
  sdram_csn  <= sd_cmd.cs_n;
  sdram_rasn <= sd_cmd.ras_n;
  sdram_casn <= sd_cmd.cas_n;
  sdram_wen  <= sd_cmd.we_n;
  sdram_dqm  <= sd_cmd.dqm;
  sdram_a    <= sd_cmd.a;
  sdram_ba   <= sd_cmd.ba;

  -- M2 peripheral bus: split DEV_PERIPH into UART/AIC/GPIO. The AIC v1 provides
  -- the interrupt controller + RTC + PIT and drives the CPU event interface.
  pmux : entity work.periph_mux(rtl)
    port map (cpu_o => cpu0_periph_dbus_o, cpu_i => cpu0_periph_dbus_i,
              uart_o => uart_dbus_o, uart_i => uart_dbus_i,
              aic_o  => aic_dbus_o,  aic_i  => aic_dbus_i,
              gpio_o => gpio_dbus_o, gpio_i => gpio_dbus_i);

  -- btn(1) drives aic.irq_i(0): the AIC's per-line edge detector turns a button
  -- press into a vectored interrupt (vector_numbers(0)=0x11). btn(0) stays reset.
  aic_irq <= (0 => btn(1), others => '0');

  -- GPIO: d_o -> LEDs, d_i <- buttons. gpio2 has no IRQ output (irq tied '0'
  -- in the entity); the button interrupt uses the AIC's own edge detector
  -- (Task 5), so gpio2 here is the memory-mapped LED/button data path.
  gpio0 : entity work.gpio2(arch)
    port map (clk => clk_cpu, rst => rst,
              db_i => gpio_dbus_o, db_o => gpio_dbus_i,
              irq => open,
              d_i => gpio_d_i, d_o => gpio_d_o, d_t => gpio_d_t);

  gpio_d_i <= x"000000" & '0' & btn;   -- buttons in d_i(6:0)

  aic0 : entity work.aic(behav)
    generic map (c_busperiod => CFG_CLK_CPU_PERIOD_NS)
    port map (clk_bus => clk_cpu, rst_i => rst,
              db_i => aic_dbus_o, db_o => aic_dbus_i,
              bstb_i => '0', back_i => '0',
              rtc_sec => aic_rtc_sec, rtc_nsec => aic_rtc_nsec,
              irq_i => aic_irq, enmi_i => '1',   -- NMI is active-low; '1' = no NMI
              event_i => cpu0_event_o_s, event_o => cpu0_event_i_s,
              reboot => open);

  -- fclk derived from the board config so the baud divisor tracks the CPU
  -- clock if the PLL setting changes (single source of truth: config.vhd).
  uart0 : entity work.uartlitedb(arch)
    generic map (bps => 115200.0, fclk => real(CFG_CLK_CPU_FREQ_HZ), intcfg => 1)
    port map (clk => clk_cpu, rst => rst,
              db_i => uart_dbus_o, db_o => uart_dbus_i,
              int => open, rx => ftdi_rxd, tx => uart_tx);

  ftdi_txd <= uart_tx;

  -- LEDs are GPIO-controlled (software drives gpio2 d_o)
  led <= gpio_d_o(7 downto 0);
end architecture;
