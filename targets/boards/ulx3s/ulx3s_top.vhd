library ieee;
use ieee.std_logic_1164.all;
use ieee.numeric_std.all;
use work.cpu2j0_pack.all;
use work.data_bus_pack.all;

entity ulx3s_top is
  port (
    clk_25mhz : in  std_logic;
    ftdi_txd  : out std_logic;   -- FPGA -> host (our TX)
    ftdi_rxd  : in  std_logic;   -- host -> FPGA (our RX)
    btn       : in  std_logic_vector(6 downto 0);
    led       : out std_logic_vector(7 downto 0));
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

  signal uart_tx : std_logic;
  signal heartbeat : unsigned(23 downto 0) := (others => '0');

  -- clkgen as a component so a configuration can pick sim (tb) vs ecp5 (synth);
  -- default binding is the last-analyzed architecture (ecp5) for synthesis.
  component clkgen
    port (clk_in : in std_logic; rst_in : in std_logic;
          clk : out std_logic; locked : out std_logic);
  end component;
begin
  ext_rst <= btn(0);  -- ULX3S btn(0) (PWR/FIRE1) as external reset

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
      cpu0_event_i => NULL_CPU_EVENT_I, cpu0_event_o => open,
      cpu0_mem_lock => open,
      cpu0_periph_dbus_i => cpu0_periph_dbus_i, cpu0_periph_dbus_o => cpu0_periph_dbus_o,
      cpu1_copro_i => NULL_COPR_I, cpu1_copro_o => open,
      cpu1_data_master_ack => open, cpu1_data_master_en => open,
      cpu1_ddr_dbus_i => (d => (others => '0'), ack => '0'), cpu1_ddr_dbus_o => open,
      cpu1_ddr_ibus_i => (d => (others => '0'), ack => '0'), cpu1_ddr_ibus_o => open,
      cpu1_event_i => NULL_CPU_EVENT_I, cpu1_event_o => open,
      cpu1_mem_lock => open,
      cpu1_periph_dbus_i => (d => (others => '0'), ack => '0'), cpu1_periph_dbus_o => open,
      cpu1eni => '0', debug_i => CPU_DEBUG_NOP, debug_o => open);

  -- M0 has no DDR: idle-respond so a stray access acks without hanging.
  cpu0_ddr_dbus_i <= loopback_bus(cpu0_ddr_dbus_o);
  cpu0_ddr_ibus_i <= (d => (others => '0'), ack => cpu0_ddr_ibus_o.en);

  uart0 : entity work.uartlitedb(arch)
    generic map (bps => 115200.0, fclk => 25.0e6, intcfg => 1)
    port map (clk => clk_cpu, rst => rst,
              db_i => cpu0_periph_dbus_o, db_o => cpu0_periph_dbus_i,
              int => open, rx => ftdi_rxd, tx => uart_tx);

  ftdi_txd <= uart_tx;

  -- liveness heartbeat (not part of the M0 gate; handy on hardware)
  process(clk_cpu) begin
    if rising_edge(clk_cpu) then heartbeat <= heartbeat + 1; end if;
  end process;
  led <= (0 => pll_locked, 1 => not rst, 7 => heartbeat(23), others => '0');
end architecture;
