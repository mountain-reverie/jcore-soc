library ieee;
use ieee.std_logic_1164.all;
use ieee.numeric_std.all;
use work.sdram_pkg.all;

-- Boot the generated gf180_j4mmu `soc` (elaborated DIRECTLY -- this target
-- has no pad_ring/PLL, see targets/asic/gf180_j4mmu/README.md) end to end in
-- GHDL: drive clk_sys/reset ourselves (no ECP5 PLL passthrough to bypass),
-- attach the shared behavioral SDRAM model (via sdram_iocells, same pattern
-- as components/sdram/tests/sdram_ctrl_tb.vhd) to sd_cmd/sd_dq_*, and decode
-- uart0_tx the same way targets/boards/ulx3s/tb/ulx3s_gen_tb.vhd does,
-- requiring the same boot banner + SPI loopback substrings (SPI loopback is
-- an internal spi2 hardware loopback mode -- see boot/files/spi.c -- so
-- spi2_miso needs no external wiring).
entity gf180_j4mmu_gen_tb is
end entity;

architecture sim of gf180_j4mmu_gen_tb is
  constant CLK_PER : time := 50 ns;            -- matches CONFIG_CLK_CPU_DIVIDE=50
                                                -- (same value as targets/boards/ulx3s)
  constant BIT_PER : time := 1000 ms / 115200; -- one UART bit at 115200 baud
  signal clk_sys : std_logic := '0';
  signal reset   : std_logic := '1';
  signal uart0_rx : std_logic := '1';
  signal uart0_tx : std_logic;
  signal gpio_di : std_logic_vector(31 downto 0) := (others => '0');
  signal gpio_do : std_logic_vector(31 downto 0);
  signal spi2_clk, spi2_mosi, spi2_miso : std_logic;
  signal spi2_cs : std_logic_vector(1 downto 0);
  signal done : boolean := false;

  -- SDRAM pins (soc <-> behavioral model, via sdram_iocells for the inout dq)
  signal sd_cmd : sdram_cmd_t;
  signal sd_dq_i, sd_dq_o : std_logic_vector(15 downto 0);
  signal sd_dq_oe : std_logic;
  signal sd_dq : std_logic_vector(15 downto 0);

  -- substring search over the decoded UART stream
  function contains(buf : string; n : integer; sub : string) return boolean is
  begin
    if n < sub'length then return false; end if;
    for i in 1 to n - sub'length + 1 loop
      if buf(i to i + sub'length - 1) = sub then return true; end if;
    end loop;
    return false;
  end function;
begin
  uut : entity work.soc(impl)
    port map (
      clk_sys   => clk_sys,
      reset     => reset,
      gpio_di   => gpio_di,
      gpio_do   => gpio_do,
      sd_cmd    => sd_cmd,
      sd_dq_i   => sd_dq_i,
      sd_dq_o   => sd_dq_o,
      sd_dq_oe  => sd_dq_oe,
      spi2_clk  => spi2_clk,
      spi2_cs   => spi2_cs,
      spi2_miso => spi2_miso,
      spi2_mosi => spi2_mosi,
      uart0_rx  => uart0_rx,
      uart0_tx  => uart0_tx);

  spi2_miso <= '1'; -- unused: "SPI LOOPBACK OK" is an internal spi2 hw loopback

  io : entity work.sdram_iocells(rtl)
    port map (dq_o => sd_dq_o, dq_oe => sd_dq_oe, dq_i => sd_dq_i, dq => sd_dq);

  mem : entity work.sdram_model(behave)
    generic map (CAS_LATENCY => 2, MEM_WORDS => 8192)
    port map (clk => clk_sys, cke => sd_cmd.cke, cs_n => sd_cmd.cs_n,
              ras_n => sd_cmd.ras_n, cas_n => sd_cmd.cas_n, we_n => sd_cmd.we_n,
              ba => sd_cmd.ba, a => sd_cmd.a, dqm => sd_cmd.dqm, dq => sd_dq);

  clk_sys <= not clk_sys after CLK_PER/2 when not done else '0';

  stim : process begin
    wait for 10 * CLK_PER; reset <= '0';
    wait;
  end process;

  -- UART receiver: decode uart0_tx into a string; succeed once the required
  -- substrings have appeared (banner + SPI loopback).
  rx : process
    variable buf : string(1 to 1024);
    variable n : integer := 0;
    variable b : std_logic_vector(7 downto 0);
  begin
    loop
      wait until uart0_tx = '0';        -- start bit
      wait for BIT_PER/2;
      for k in 0 to 7 loop
        wait for BIT_PER;
        b(k) := uart0_tx;               -- LSB first
      end loop;
      wait for BIT_PER;                 -- stop bit
      if n < buf'length then
        n := n + 1;
        buf(n) := character'val(to_integer(unsigned(b)));
      end if;
      if contains(buf, n, "HS-2J0 SH2 ROM") and contains(buf, n, "SPI LOOPBACK OK") then
        report "gf180_j4mmu_gen_tb PASSED: bootloader started and SPI loopback verified"
          severity note;
        done <= true;
        wait;
      end if;
    end loop;
  end process;

  watchdog : process begin
    wait for 18 ms;
    assert done report "TIMEOUT: required UART output not seen" severity failure;
    wait;
  end process;
end architecture;
