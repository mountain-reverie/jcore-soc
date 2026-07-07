library ieee;
use ieee.std_logic_1164.all;
use ieee.numeric_std.all;

-- Task 1 (spi_flash_fill) unit test: t1_fill drives a behavioral SPI flash
-- model (decoding the Fast-Read 0x0B command straight off spi_flash_fill's
-- d_* pins -- no ice_spi_io/SB_IO in the loop, matching the note that
-- ice_spi_io is only instantiated at the cpus level) and asserts that a full
-- 4 KB page fill lands correctly in the frame EBR write port: big-endian
-- byte->word packing and the expected fr_addr for the given frame index.
entity spi_page_cache_tb is end entity;

architecture sim of spi_page_cache_tb is
  constant CLK_PER : time := 20 ns;

  signal clk   : std_logic := '0';
  signal rst   : std_logic := '1';
  signal start : std_logic := '0';
  signal page  : std_logic_vector(7 downto 0) := (others => '0');
  signal frame : std_logic_vector(1 downto 0) := (others => '0');
  signal busy  : std_logic;
  signal done  : std_logic;

  signal fr_we   : std_logic;
  signal fr_addr : std_logic_vector(11 downto 0);
  signal fr_data : std_logic_vector(31 downto 0);

  signal d_cs_n  : std_logic;
  signal d_sck   : std_logic;
  signal d_mosi  : std_logic;
  signal d_miso  : std_logic := '0';

  signal first_word : std_logic_vector(31 downto 0) := (others => '0');
  signal first_addr : std_logic_vector(11 downto 0) := (others => '0');
  signal first_seen : boolean := false;

  signal last_word  : std_logic_vector(31 downto 0) := (others => '0');
  signal last_addr  : std_logic_vector(11 downto 0) := (others => '0');
  signal we_count   : natural := 0;

  signal done_rises : natural := 0;
  signal done_prev  : std_logic := '0';

  signal t1_done : boolean := false;

  constant FLASH_BASE : std_logic_vector(23 downto 0) := x"100000";

  -- flash_mem(k) := k mod 256, per the task's ambiguity-resolution note.
  -- Modeled as a function (rather than a 16 MB array) since only the
  -- low-order byte of the address matters.
  function flash_mem(k : natural) return std_logic_vector is
  begin
    return std_logic_vector(to_unsigned(k mod 256, 8));
  end function;

begin

  clk <= not clk after CLK_PER / 2 when not t1_done else '0';

  dut : entity work.spi_flash_fill
    generic map (FLASH_BASE => FLASH_BASE)
    port map (
      clk => clk, rst => rst,
      start => start, page => page, frame => frame,
      busy => busy, done => done,
      fr_we => fr_we, fr_addr => fr_addr, fr_data => fr_data,
      d_cs_n => d_cs_n, d_sck => d_sck, d_mosi => d_mosi, d_miso => d_miso);

  -- Behavioral SPI flash model (mode 0): while cs_n is low, sample mosi on
  -- sck rising edges. Decode 8 (cmd) + 24 (addr) bits, then 8 dummy bits,
  -- then drive flash_mem(addr+i) MSB-first on miso, i incrementing each byte.
  flash_model : process
    variable shreg   : std_logic_vector(31 downto 0);
    variable bitcnt  : natural;
    variable addr    : natural;
    variable tx_byte : std_logic_vector(7 downto 0);
    variable byte_i  : natural;
  begin
    d_miso <= '0';
    -- wait for cs_n to go low
    wait until d_cs_n = '0';

    -- shift in cmd+addr (32 bits) on sck rising edges
    shreg := (others => '0');
    for i in 0 to 31 loop
      wait until rising_edge(d_sck);
      shreg := shreg(30 downto 0) & d_mosi;
    end loop;
    addr := to_integer(unsigned(shreg(23 downto 0)));

    -- 8 dummy clocks
    for i in 0 to 7 loop
      wait until rising_edge(d_sck);
    end loop;

    -- stream bytes until cs_n deasserted
    byte_i := 0;
    loop
      tx_byte := flash_mem(addr + byte_i);
      for b in 7 downto 0 loop
        if d_cs_n = '1' then
          exit;
        end if;
        d_miso <= tx_byte(b);
        wait until rising_edge(d_sck) or d_cs_n = '1';
        if d_cs_n = '1' then
          exit;
        end if;
      end loop;
      byte_i := byte_i + 1;
      if d_cs_n = '1' then
        exit;
      end if;
    end loop;
    wait;
  end process;

  -- capture the first frame write
  capture : process (clk)
  begin
    if rising_edge(clk) then
      if fr_we = '1' then
        if not first_seen then
          first_word <= fr_data;
          first_addr <= fr_addr;
          first_seen <= true;
        end if;
        last_word <= fr_data;
        last_addr <= fr_addr;
        we_count  <= we_count + 1;
      end if;
      -- count rising edges of `done`
      if done = '1' and done_prev = '0' then
        done_rises <= done_rises + 1;
      end if;
      done_prev <= done;
    end if;
  end process;

  t1_fill : process
  begin
    rst <= '1';
    wait for CLK_PER * 4;
    rst <= '0';
    wait for CLK_PER * 4;

    page  <= x"02";
    frame <= "01";
    start <= '1';
    wait for CLK_PER;
    start <= '0';

    wait until done = '1' for 2 ms;
    assert done = '1' report "spi_flash_fill did not complete (done never asserted)" severity failure;
    -- let the clk-edge capture process register the final fr_we/done edge
    wait until rising_edge(clk);
    wait until rising_edge(clk);

    assert first_word = x"00010203" report "fill word0 packing" severity failure;
    assert first_addr = std_logic_vector(to_unsigned(1024, 12)) report "fill addr" severity failure;

    -- last word (index 1023): flash bytes at FLASH_BASE + (page<<12) + 1023*4
    -- = 0x102FFC..0x102FFF -> (k mod 256) = FC,FD,FE,FF, packed big-endian.
    assert last_word = x"FCFDFEFF" report "fill last-word packing" severity failure;
    -- frame "01" -> 1*1024 + 1023 = 2047
    assert last_addr = std_logic_vector(to_unsigned(2047, 12)) report "fill last addr" severity failure;

    -- exactly 1024 word writes (one per 32-bit word of the 4 KB page)
    assert we_count = 1024 report "fill fr_we pulse count" severity failure;

    -- done pulsed exactly once, and busy returned low
    assert done_rises = 1 report "done rose exactly once, got " & integer'image(done_rises) severity failure;
    assert busy = '0' report "busy returned to 0 after fill" severity failure;

    report "Test Passed t1_fill" severity note;
    t1_done <= true;
    wait;
  end process;

end architecture;
