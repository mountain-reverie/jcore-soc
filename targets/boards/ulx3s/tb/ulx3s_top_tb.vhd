library ieee;
use ieee.std_logic_1164.all;
use ieee.numeric_std.all;

entity ulx3s_top_tb is end entity;

architecture sim of ulx3s_top_tb is
  constant CLK_PER : time := 40 ns;            -- 25 MHz
  constant BIT_PER : time := 1000 ms / 115200; -- one UART bit at 115200 baud
  signal clk_25mhz : std_logic := '0';
  signal ftdi_txd : std_logic;
  signal ftdi_rxd : std_logic := '1';
  signal btn : std_logic_vector(6 downto 0) := (0 => '1', others => '0'); -- reset asserted
  signal led : std_logic_vector(7 downto 0);
  signal done : boolean := false;
begin
  -- Direct entity instantiation: default binding selects clkgen(sim) (the only
  -- clkgen arch the sim flow analyzes) and binds the nested uart, with no
  -- configuration (configurations suppress default binding for unconfigured
  -- nested instances, leaving uart unbound).
  uut : entity work.ulx3s_top(rtl)
    port map (clk_25mhz => clk_25mhz, ftdi_txd => ftdi_txd, ftdi_rxd => ftdi_rxd,
              btn => btn, led => led);

  clk_25mhz <= not clk_25mhz after CLK_PER/2 when not done else '0';

  -- release reset after a few cycles
  rel : process begin
    wait for 10 * CLK_PER; btn(0) <= '0'; wait;
  end process;

  -- UART receiver: sample ftdi_txd, decode bytes, check the banner prefix
  rx : process
    constant EXPECT : string := "J2 on ULX3S";
    variable b : std_logic_vector(7 downto 0);
    variable c : character;
  begin
    for i in EXPECT'range loop
      -- wait for start bit (falling edge of idle-high line)
      wait until ftdi_txd = '0';
      wait for BIT_PER/2;            -- center of start bit
      for k in 0 to 7 loop
        wait for BIT_PER;
        b(k) := ftdi_txd;            -- LSB first
      end loop;
      wait for BIT_PER;             -- stop bit
      c := character'val(to_integer(unsigned(b)));
      assert c = EXPECT(i)
        report "banner mismatch at " & integer'image(i) & ": got '" & c &
               "' want '" & EXPECT(i) & "'" severity failure;
    end loop;
    report "ulx3s_top_tb PASSED: banner prefix decoded" severity note;
    done <= true;
    wait;
  end process;

  -- Fail loudly if no banner ever transmits (otherwise the rx process just
  -- waits and the run ends silently at --stop-time, a false pass).
  watchdog : process begin
    wait for 2 ms;
    assert done report "TIMEOUT: no banner decoded within 2 ms" severity failure;
    wait;
  end process;
end architecture;
