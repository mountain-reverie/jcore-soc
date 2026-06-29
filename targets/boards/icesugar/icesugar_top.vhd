library ieee;
use ieee.std_logic_1164.all;

-- iCESugar (iCE40 UP5K) top level. Hand-written board top, analogous to ULX3S's
-- ulx3s_top.vhd but far simpler: no PLL (the J1 runs directly at the 12 MHz
-- oscillator) and no SDRAM (all memory is on-chip EBR via bootram_infer). It
-- wires the 12 MHz clock/reset generator (ice_clkgen) to the generated SoC
-- (entity work.soc, whose internal `cpus` instance is bound to one_cpu_ebr ->
-- cpu_synth_j1 by the generated soc_cpus_config), routes the FTDI UART, and
-- drives the active-low RGB LED from gpio_do(2 downto 0).
entity icesugar_top is
  port (
    clk     : in  std_logic;   -- 12 MHz oscillator
    ser_rx  : in  std_logic;   -- host -> FPGA
    ser_tx  : out std_logic;   -- FPGA -> host
    ledr_n  : out std_logic;   -- active-low RGB LED, red
    ledg_n  : out std_logic;   -- active-low RGB LED, green
    ledb_n  : out std_logic);  -- active-low RGB LED, blue
end entity;

architecture rtl of icesugar_top is
  signal clk_cpu   : std_logic;
  signal rst       : std_logic;
  signal gpio_do   : std_logic_vector(7 downto 0);
  signal uart0_rx  : std_logic;
  signal uart0_tx  : std_logic;
begin
  clkgen : entity work.ice_clkgen(rtl)
    port map (clk_in => clk, clk_out => clk_cpu, rst_out => rst);

  -- The generated SoC: CPU (one_cpu_ebr / cpu_synth_j1) + EBR boot RAM + the
  -- uartlite/gpio2 peripheral bus. The cpus configuration is bound inside soc.
  soc0 : entity work.soc(impl)
    port map (
      clk_sys  => clk_cpu,
      reset    => rst,
      gpio_do  => gpio_do,
      uart0_rx => uart0_rx,
      uart0_tx => uart0_tx);

  uart0_rx <= ser_rx;
  ser_tx   <= uart0_tx;

  -- RGB LED is common-anode (active-low): a '1' GPIO output lights the LED.
  ledr_n <= not gpio_do(0);
  ledg_n <= not gpio_do(1);
  ledb_n <= not gpio_do(2);
end architecture;
