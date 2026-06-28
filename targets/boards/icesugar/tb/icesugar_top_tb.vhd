library ieee;
use ieee.std_logic_1164.all;
use ieee.numeric_std.all;

-- Top-level sim of the iCESugar EBR-only J1 SoC: drive the 12 MHz oscillator,
-- let ice_clkgen's power-on reset release, capture ser_tx, UART-decode it, and
-- assert the boot banner appears within a timeout. Mirrors ULX3S's
-- ulx3s_top_tb but with no SDRAM model and no PLL (clk is the CPU clock).
entity icesugar_top_tb is end entity;

architecture sim of icesugar_top_tb is
  constant CLK_PER : time := 1 sec / 12_000_000;   -- 12 MHz oscillator
  constant BIT_PER : time := 1 sec / 115200;       -- one UART bit at ~115200 baud
  signal clk    : std_logic := '0';
  signal ser_rx : std_logic := '1';
  signal ser_tx : std_logic;
  signal ledr_n, ledg_n, ledb_n : std_logic;
  signal done   : boolean := false;

  function contains(buf : string; n : integer; sub : string) return boolean is
  begin
    if n < sub'length then return false; end if;
    for i in 1 to n - sub'length + 1 loop
      if buf(i to i + sub'length - 1) = sub then return true; end if;
    end loop;
    return false;
  end function;
begin
  uut : entity work.icesugar_top(rtl)
    port map (clk => clk, ser_rx => ser_rx, ser_tx => ser_tx,
              ledr_n => ledr_n, ledg_n => ledg_n, ledb_n => ledb_n);

  clk <= not clk after CLK_PER/2 when not done else '0';

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
      if contains(buf, n, "J1 on iCESugar: hello") and
         contains(buf, n, "GPIO") then
        report "icesugar_top_tb PASSED: J1 booted and emitted the UART banner"
          severity note;
        done <= true;
        wait;
      end if;
    end loop;
  end process;

  watchdog : process begin
    wait for 5 ms;
    assert done report "TIMEOUT: banner not seen on ser_tx" severity failure;
    wait;
  end process;
end architecture;
