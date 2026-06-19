library ieee;
use ieee.std_logic_1164.all;
use ieee.numeric_std.all;
use work.sdram_pkg.all;

entity ulx3s_gen_tb is end entity;

architecture sim of ulx3s_gen_tb is
  constant CLK_PER : time := 50 ns;            -- 20 MHz (sim bypasses the PLL;
                                               -- feed clk_cpu's post-PLL rate)
  constant BIT_PER : time := 1000 ms / 115200; -- one UART bit at 115200 baud
  signal clk_25mhz : std_logic := '0';
  signal ftdi_txd : std_logic;
  signal ftdi_rxd : std_logic := '1';
  signal btn : std_logic_vector(6 downto 0) := (0 => '1', others => '0'); -- reset asserted
  signal led : std_logic_vector(7 downto 0);
  signal done : boolean := false;
  signal gpio_seen : boolean := false;   -- set by rx once "GPIO" is decoded

  -- SDRAM pins (top <-> behavioral model)
  signal s_clk, s_cke, s_csn, s_rasn, s_casn, s_wen : std_logic;
  signal s_dqm : std_logic_vector(1 downto 0);
  signal s_a   : std_logic_vector(12 downto 0);
  signal s_ba  : std_logic_vector(1 downto 0);
  signal s_d   : std_logic_vector(15 downto 0);

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
  -- socgen-generated FPGA top: per-bit pin_<net> ports bundled here into the
  -- vectors the sdram_model + checks use.
  uut : entity work.pad_ring(impl)
    port map (pin_clk_25mhz => clk_25mhz, pin_ftdi_txd => ftdi_txd, pin_ftdi_rxd => ftdi_rxd,
              pin_btn0 => btn(0), pin_btn1 => btn(1), pin_btn2 => btn(2), pin_btn3 => btn(3),
              pin_btn4 => btn(4), pin_btn5 => btn(5), pin_btn6 => btn(6),
              pin_led0 => led(0), pin_led1 => led(1), pin_led2 => led(2), pin_led3 => led(3),
              pin_led4 => led(4), pin_led5 => led(5), pin_led6 => led(6), pin_led7 => led(7),
              pin_sdram_clk => s_clk, pin_sdram_cke => s_cke, pin_sdram_csn => s_csn,
              pin_sdram_rasn => s_rasn, pin_sdram_casn => s_casn, pin_sdram_wen => s_wen,
              pin_sdram_dqm0 => s_dqm(0), pin_sdram_dqm1 => s_dqm(1),
              pin_sdram_a0 => s_a(0), pin_sdram_a1 => s_a(1), pin_sdram_a2 => s_a(2),
              pin_sdram_a3 => s_a(3), pin_sdram_a4 => s_a(4), pin_sdram_a5 => s_a(5),
              pin_sdram_a6 => s_a(6), pin_sdram_a7 => s_a(7), pin_sdram_a8 => s_a(8),
              pin_sdram_a9 => s_a(9), pin_sdram_a10 => s_a(10), pin_sdram_a11 => s_a(11),
              pin_sdram_a12 => s_a(12),
              pin_sdram_ba0 => s_ba(0), pin_sdram_ba1 => s_ba(1),
              pin_sdram_d0 => s_d(0), pin_sdram_d1 => s_d(1), pin_sdram_d2 => s_d(2),
              pin_sdram_d3 => s_d(3), pin_sdram_d4 => s_d(4), pin_sdram_d5 => s_d(5),
              pin_sdram_d6 => s_d(6), pin_sdram_d7 => s_d(7), pin_sdram_d8 => s_d(8),
              pin_sdram_d9 => s_d(9), pin_sdram_d10 => s_d(10), pin_sdram_d11 => s_d(11),
              pin_sdram_d12 => s_d(12), pin_sdram_d13 => s_d(13), pin_sdram_d14 => s_d(14),
              pin_sdram_d15 => s_d(15));

  mem : entity work.sdram_model(behave)
    generic map (CAS_LATENCY => 2, MEM_WORDS => 8192)
    port map (clk => s_clk, cke => s_cke, cs_n => s_csn, ras_n => s_rasn,
              cas_n => s_casn, we_n => s_wen, ba => s_ba, a => s_a, dqm => s_dqm, dq => s_d);

  clk_25mhz <= not clk_25mhz after CLK_PER/2 when not done else '0';

  -- single driver for btn: release reset, then once the program reaches the
  -- GPIO stage (and has armed the button IRQ) press btn(1) -> rising edge on
  -- aic.irq_i(0) -> the GPIO interrupt fires.
  stim : process begin
    wait for 10 * CLK_PER; btn(0) <= '0';
    wait until gpio_seen;
    wait for 5 us;            -- let the program arm AIC_ILEVEL after printing GPIO
    btn(1) <= '1';
    wait for 50 us;
    btn(1) <= '0';
    wait;
  end process;

  -- UART receiver: decode ftdi_txd into a string; succeed once the required
  -- substring(s) have all appeared. (M1b Stages add SDRAM TEST PASS / FROM SDRAM.)
  rx : process
    variable buf : string(1 to 1024);
    variable n : integer := 0;
    variable b : std_logic_vector(7 downto 0);
  begin
    loop
      wait until ftdi_txd = '0';        -- start bit
      wait for BIT_PER/2;
      for k in 0 to 7 loop
        wait for BIT_PER;
        b(k) := ftdi_txd;               -- LSB first
      end loop;
      wait for BIT_PER;                 -- stop bit
      if n < buf'length then
        n := n + 1;
        buf(n) := character'val(to_integer(unsigned(b)));
      end if;
      assert not contains(buf, n, "SDRAM TEST FAIL")
        report "ulx3s_gen_tb FAILED: SDRAM memory test reported FAIL" severity failure;
      if contains(buf, n, "GPIO") then gpio_seen <= true; end if;
      if contains(buf, n, "J2 on ULX3S") and contains(buf, n, "SDRAM TEST PASS")
         and contains(buf, n, "FROM SDRAM")
         and contains(buf, n, "TICK") and contains(buf, n, "RTC")
         and contains(buf, n, "GPIO") and contains(buf, n, "BTN") then
        report "ulx3s_gen_tb PASSED: banner + SDRAM + TICK + RTC + GPIO + BTN decoded"
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
