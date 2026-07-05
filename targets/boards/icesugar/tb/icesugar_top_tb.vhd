library ieee;
use ieee.std_logic_1164.all;
use ieee.numeric_std.all;

-- Top-level sim of the iCESugar EBR-only J1 SoC: drive the 12 MHz oscillator,
-- let ice_clkgen's power-on reset release, capture ser_tx, UART-decode it, and
-- assert the boot banner appears within a timeout. Mirrors ULX3S's
-- ulx3s_top_tb but with no SDRAM model and no PLL (clk is the CPU clock).
--
-- The 'eth' device is now a spi2 master driving a W5500 (WIZ850io PMOD) --
-- see components/emac/w5500_model.vhd for the behavioral SPI-slave model
-- instantiated below on pad_ring's pin_w5500_* pins. Once banner.c's
-- w5500_init_ping() programs MAC/IP/subnet/gateway, the chip would answer
-- ARP/ICMP entirely in hardware (not modeled here); this tb instead asserts
-- directly on the model's captured SHAR/SIPR registers.
entity icesugar_top_tb is end entity;

architecture sim of icesugar_top_tb is
  constant CLK_PER : time := 1 sec / 12_000_000;   -- 12 MHz oscillator
  constant BIT_PER : time := 1 sec / 115200;       -- one UART bit at ~115200 baud
  signal clk    : std_logic := '0';
  signal ser_rx : std_logic := '1';
  signal ser_tx : std_logic;
  signal ledr_n, ledg_n, ledb_n : std_logic;
  signal w5500_cs, w5500_sclk, w5500_mosi, w5500_miso : std_logic;
  signal shar_out : std_logic_vector(47 downto 0);
  signal sipr_out : std_logic_vector(31 downto 0);
  signal done     : boolean := false;
  signal spram_ok : boolean := false;   -- boot printed "SPRAM MEMTEST OK"
  signal w5500_ok : boolean := false;
  signal ds3231_model_ok : boolean := false;  -- model regfile matches expected time
  signal ds3231_ok       : boolean := false;  -- + driver's own read-back matched (UART)

  -- i2c pads: pad_ring's pin_i2c_scl/pin_i2c_sda (both "out" at the pad_ring
  -- boundary, forwarding the internal open-drain i2c_scl_pad/i2c_sda_pad
  -- net) wired straight to ds3231_model. Both this entity's "out" port and
  -- the model's inout port drive the same resolved std_logic net, which is
  -- exactly how the real open-drain bus (external pull-up + multiple
  -- drivers) behaves. The pull-up itself is modelled in
  -- components/emac/sb_io_sim.vhd: SB_IO's PULLUP generic drives a weak
  -- 'H' when released instead of 'Z', so a released line resolves high
  -- without needing a separate pull-up process here.
  signal i2c_scl, i2c_sda : std_logic;
  signal ds3231_sec, ds3231_min, ds3231_hour  : std_logic_vector(7 downto 0);
  signal ds3231_day, ds3231_date, ds3231_month, ds3231_year : std_logic_vector(7 downto 0);
  signal rtc_sqw     : std_logic;      -- ds3231_model's fast-pulse SQW -> aic0 irq_i(1)
  signal aic_irq_ok  : boolean := false;  -- "AIC PASS" seen on the UART (tick_count>0)

  -- Expected values programmed by banner.c's w5500_init_ping(): SHAR
  -- 02:00:00:00:00:01, SIPR 192.168.1.10 (0xC0A8010A).
  constant EXPECT_SHAR : std_logic_vector(47 downto 0) := x"020000000001";
  constant EXPECT_SIPR : std_logic_vector(31 downto 0) := x"C0A8010A";

  -- Expected time programmed by banner.c's ds3231_init(): 2024-01-02
  -- 03:04:05, all BCD (DS3231 register map: sec/min/hour/day/date/month/year).
  constant EXPECT_SEC   : std_logic_vector(7 downto 0) := x"05";
  constant EXPECT_MIN   : std_logic_vector(7 downto 0) := x"04";
  constant EXPECT_HOUR  : std_logic_vector(7 downto 0) := x"03";
  constant EXPECT_DATE  : std_logic_vector(7 downto 0) := x"02";
  constant EXPECT_MONTH : std_logic_vector(7 downto 0) := x"01";
  constant EXPECT_YEAR  : std_logic_vector(7 downto 0) := x"24";

  function contains(buf : string; n : integer; sub : string) return boolean is
  begin
    if n < sub'length then return false; end if;
    for i in 1 to n - sub'length + 1 loop
      if buf(i to i + sub'length - 1) = sub then return true; end if;
    end loop;
    return false;
  end function;
begin
  uut : entity work.pad_ring(impl)
    port map (pin_clk => clk, pin_ser_rx => ser_rx, pin_ser_tx => ser_tx,
              pin_ledr_n => ledr_n, pin_ledg_n => ledg_n, pin_ledb_n => ledb_n,
              pin_w5500_cs => w5500_cs, pin_w5500_sclk => w5500_sclk,
              pin_w5500_mosi => w5500_mosi, pin_w5500_miso => w5500_miso,
              pin_i2c_pad0 => i2c_scl, pin_i2c_pad1 => i2c_sda,
              -- rtc_sqw is now driven by ds3231_model's fast-pulse SQW output
              -- below (real hardware pulses far too slowly to exercise the
              -- AIC irq_i(1) path in sim). w5500_int stays out of scope for
              -- this tb (the W5500's own interrupt line isn't exercised
              -- here): tie idle-high, matching its active-low idle level on
              -- real hardware.
              pin_rtc_sqw => rtc_sqw, pin_w5500_int => '1');

  clk <= not clk after CLK_PER/2 when not done else '0';

  -- DS3231 RTC sim model: behavioral I2C slave at 0x68 on the bit-banged
  -- SCL/SDA pads. banner.c's ds3231_init() writes a known time then reads
  -- it back; this model's regfile is checked directly below (independent
  -- of what the CPU thinks it read back) for a true end-to-end check.
  ds3231 : entity work.ds3231_model(sim)
    port map (
      scl       => i2c_scl,
      sda       => i2c_sda,
      sqw       => rtc_sqw,
      reg_sec   => ds3231_sec,
      reg_min   => ds3231_min,
      reg_hour  => ds3231_hour,
      reg_day   => ds3231_day,
      reg_date  => ds3231_date,
      reg_month => ds3231_month,
      reg_year  => ds3231_year
    );

  -- Poll the model's regfile once the boot has printed the DS3231 line
  -- (banner.c prints "DS3231 PASS"/"FAIL" right after ds3231_init()
  -- returns, so the I2C transaction has completed by then).
  ds3231_check : process
  begin
    wait until spram_ok;
    wait until (ds3231_sec = EXPECT_SEC and ds3231_min = EXPECT_MIN and
                ds3231_hour = EXPECT_HOUR and ds3231_date = EXPECT_DATE and
                ds3231_month = EXPECT_MONTH and ds3231_year = EXPECT_YEAR)
               for 50 ms;
    assert (ds3231_sec = EXPECT_SEC and ds3231_min = EXPECT_MIN and
            ds3231_hour = EXPECT_HOUR and ds3231_date = EXPECT_DATE and
            ds3231_month = EXPECT_MONTH and ds3231_year = EXPECT_YEAR)
      report "icesugar_top_tb: DS3231 model regfile did not match the time banner.c wrote"
      severity error;
    if (ds3231_sec = EXPECT_SEC and ds3231_min = EXPECT_MIN and
        ds3231_hour = EXPECT_HOUR and ds3231_date = EXPECT_DATE and
        ds3231_month = EXPECT_MONTH and ds3231_year = EXPECT_YEAR) then
      report "PASS: DS3231 model regfile holds the time banner.c wrote" severity note;
      ds3231_model_ok <= true;
    end if;
    wait;
  end process;

  -- W5500 sim model: watches the SPI bus banner.c's w5500_init_ping()
  -- drives, captures SHAR/SIPR as it's written, and exposes them for the
  -- assertion below.
  w5500 : entity work.w5500_model(sim)
    port map (
      clk      => clk,
      spi_sclk => w5500_sclk,
      spi_mosi => w5500_mosi,
      spi_miso => w5500_miso,
      spi_cs   => w5500_cs,
      shar_out => shar_out,
      sipr_out => sipr_out
    );

  -- Poll the model's captured registers once the boot has printed
  -- "W5500 INIT OK" (banner.c prints this right after w5500_init_ping()
  -- returns, so all SPI frames have completed by then).
  w5500_check : process
  begin
    wait until spram_ok;
    wait until shar_out = EXPECT_SHAR and sipr_out = EXPECT_SIPR
               for 50 ms;
    assert shar_out = EXPECT_SHAR and sipr_out = EXPECT_SIPR
      report "icesugar_top_tb: W5500 model did not observe expected SHAR/SIPR"
      severity error;
    if shar_out = EXPECT_SHAR and sipr_out = EXPECT_SIPR then
      report "PASS: W5500 programmed with correct MAC+IP" severity note;
      w5500_ok <= true;
    end if;
    wait;
  end process;

  -- UART receiver: decode ser_tx into a string; succeed once the banner has
  -- appeared. Sample at the bit centre, LSB first, 8N1.
  rx : process
    variable buf : string(1 to 1024);
    variable n : integer := 0;
    variable b : std_logic_vector(7 downto 0);
  begin
    loop
      wait until ser_tx = '0';          -- start bit
      wait for BIT_PER/2;
      for k in 0 to 7 loop
        wait for BIT_PER;
        b(k) := ser_tx;                 -- LSB first
      end loop;
      wait for BIT_PER;                 -- stop bit
      if n < buf'length then
        n := n + 1;
        buf(n) := character'val(to_integer(unsigned(b)));
      end if;
      if contains(buf, n, "SPRAM MEMTEST OK") and not spram_ok then
        spram_ok <= true;
      elsif contains(buf, n, "SPRAM MEMTEST FAIL") then
        report "icesugar_top_tb FAILED: SPRAM MEMTEST FAIL seen"
          severity failure;
        wait;
      elsif contains(buf, n, "DS3231 PASS") and not ds3231_ok then
        report "PASS: DS3231 driver round-trip (its own read-back matched)" severity note;
        ds3231_ok <= true;
      elsif contains(buf, n, "DS3231 FAIL") then
        report "icesugar_top_tb FAILED: DS3231 FAIL seen (driver read-back mismatch)"
          severity failure;
        wait;
      elsif contains(buf, n, "AIC PASS") and not aic_irq_ok then
        report "PASS: AIC interrupt delivered + serviced" severity note;
        aic_irq_ok <= true;
      elsif contains(buf, n, "AIC FAIL") then
        report "icesugar_top_tb FAILED: AIC FAIL seen (irq_tick_count stayed 0 -- interrupt never fired/serviced)"
          severity failure;
        wait;
      end if;
    end loop;
  end process;

  -- PASS gate: kept OUT of the UART-decoder loop so the decoder keeps
  -- consuming/reporting UART while we wait for the eth events (otherwise the
  -- decoder would block on the eth waits and stop showing later output).
  pass_gate : process begin
    -- Guard each wait: `wait until X` only fires on a future transition, so a
    -- signal that is ALREADY true would otherwise block this process forever.
    if not spram_ok then wait until spram_ok; end if;
    if not w5500_ok then wait until w5500_ok; end if;
    if not ds3231_model_ok then wait until ds3231_model_ok; end if;
    if not ds3231_ok then wait until ds3231_ok; end if;
    if not aic_irq_ok then wait until aic_irq_ok; end if;
    report "icesugar_top_tb PASSED: FROM SPRAM + SPRAM MEMTEST OK + W5500 programmed + DS3231 round trip + AIC interrupt delivered"
      severity note;
    done <= true;
    wait;
  end process;

  watchdog : process begin
    -- The 128 KB SPRAM memtest runs to ~139 ms of sim time before
    -- w5500_init_ping()/ds3231_init() run; give it comfortable headroom
    -- (done is set the moment all markers hold, so a healthy run ends well
    -- before this).
    wait for 260 ms;
    assert done report "TIMEOUT: SPRAM MEMTEST OK / W5500 programmed / DS3231 round trip not seen" severity failure;
    wait;
  end process;
end architecture;
