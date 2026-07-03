library ieee;
use ieee.std_logic_1164.all;
use ieee.numeric_std.all;

entity eth_rx_tb is
end entity;

architecture sim of eth_rx_tb is

  constant CLK_ETH_PERIOD : time := 25 ns;   -- 40 MHz
  constant CLK_PERIOD     : time := 83333 ps; -- ~12 MHz
  constant HALF_BIT       : time := 50 ns;   -- 100 ns bit period / 2

  signal clk          : std_logic := '0';
  signal clk_eth       : std_logic := '0';
  signal rst          : std_logic := '1';
  signal rx_in        : std_logic := '0';
  signal rd_word_addr : unsigned(9 downto 0) := (others => '0');
  signal rd_word      : std_logic_vector(31 downto 0);
  signal frame_ready  : std_logic;
  signal rx_len       : unsigned(11 downto 0);
  signal ack          : std_logic := '0';
  signal overrun      : std_logic;
  signal rx_irq       : std_logic;

  signal sim_done : boolean := false;

  type byte_arr is array (natural range <>) of std_logic_vector(7 downto 0);

  -- Frame body driven after preamble+SFD: dest MAC (6) + src MAC (6) +
  -- ethertype (2) + a few payload bytes + 4 FCS bytes.
  constant FBODY : byte_arr := (
    x"FF", x"FF", x"FF", x"FF", x"FF", x"FF",  -- dest MAC (broadcast)
    x"02", x"00", x"00", x"00", x"00", x"01",  -- src MAC
    x"08", x"00",                              -- ethertype (IPv4)
    x"DE", x"AD", x"BE", x"EF", x"CA", x"FE",  -- payload
    x"12", x"34", x"56", x"78");               -- FCS (dummy, framer doesn't check)

  constant PREAMBLE_SFD : byte_arr := (
    x"55", x"55", x"55", x"55", x"55", x"55", x"55", x"D5");

  procedure send_frame(signal rx_in : out std_logic) is
    variable b : std_logic;
  begin
    for byte_i in PREAMBLE_SFD'range loop
      for bit_i in 0 to 7 loop
        b := PREAMBLE_SFD(byte_i)(bit_i);
        rx_in <= not b;
        wait for HALF_BIT;
        rx_in <= b;
        wait for HALF_BIT;
      end loop;
    end loop;
    for byte_i in FBODY'range loop
      for bit_i in 0 to 7 loop
        b := FBODY(byte_i)(bit_i);
        rx_in <= not b;
        wait for HALF_BIT;
        rx_in <= b;
        wait for HALF_BIT;
      end loop;
    end loop;
    rx_in <= '0';
  end procedure;

begin

  dut: entity work.eth_rx
    generic map (BUF_WORDS => 512)
    port map (
      clk          => clk,
      clk_eth       => clk_eth,
      rst          => rst,
      rx_in        => rx_in,
      rd_word_addr => rd_word_addr,
      rd_word      => rd_word,
      frame_ready  => frame_ready,
      rx_len       => rx_len,
      ack          => ack,
      overrun      => overrun,
      rx_irq       => rx_irq);

  clk_eth <= not clk_eth after CLK_ETH_PERIOD / 2 when not sim_done else '0';
  clk     <= not clk     after CLK_PERIOD / 2     when not sim_done else '0';

  stim: process
  begin
    rst   <= '1';
    rx_in <= '0';
    wait for 300 ns;
    rst <= '0';
    wait for 300 ns;

    -- Frame 1
    send_frame(rx_in);
    -- idle long enough for carrier to drop (>200ns silence), for the
    -- frame-complete CDC to reach the clk domain, and for the checker to
    -- read back + ack frame 1 before frame 2 arrives.
    wait for 20 us;

    -- Frame 2
    send_frame(rx_in);
    wait for 5 us;

    wait;
  end process;

  check: process
    variable exp_word  : std_logic_vector(31 downto 0);
    variable byte0, byte1, byte2, byte3 : std_logic_vector(7 downto 0);
    variable nwords     : integer;
    variable rd_bytes    : byte_arr(0 to FBODY'length - 1);
    variable idx         : integer;
    variable any_fail    : boolean := false;
  begin
    wait until rst = '0';

    -- ---------------------------------------------------------------
    -- Frame 1: wait for frame_ready, check rx_len, read back buffer,
    -- compare against FBODY, ack, confirm frame_ready deasserts.
    -- ---------------------------------------------------------------
    wait until frame_ready = '1' for 50 us;
    assert frame_ready = '1'
      report "eth_rx_tb: frame_ready never asserted (frame 1)" severity error;
    if frame_ready /= '1' then
      any_fail := true;
    end if;

    assert to_integer(rx_len) = FBODY'length
      report "eth_rx_tb: rx_len mismatch (frame 1): got " &
             integer'image(to_integer(rx_len)) & " expected " &
             integer'image(FBODY'length)
      severity error;
    if to_integer(rx_len) /= FBODY'length then
      any_fail := true;
    end if;

    assert overrun = '0'
      report "eth_rx_tb: unexpected overrun after frame 1" severity error;

    -- read back words covering FBODY'length bytes
    nwords := (FBODY'length + 3) / 4;
    for w in 0 to nwords - 1 loop
      rd_word_addr <= to_unsigned(w, 10);
      wait until rising_edge(clk);
      wait until rising_edge(clk); -- registered read: data valid one cycle later
      exp_word := rd_word;
      byte0 := exp_word(31 downto 24);
      byte1 := exp_word(23 downto 16);
      byte2 := exp_word(15 downto  8);
      byte3 := exp_word( 7 downto  0);
      idx := w * 4;
      if idx     <= FBODY'high then rd_bytes(idx)     := byte0; end if;
      if idx + 1 <= FBODY'high then rd_bytes(idx + 1) := byte1; end if;
      if idx + 2 <= FBODY'high then rd_bytes(idx + 2) := byte2; end if;
      if idx + 3 <= FBODY'high then rd_bytes(idx + 3) := byte3; end if;
    end loop;

    for i in FBODY'range loop
      assert rd_bytes(i) = FBODY(i)
        report "eth_rx_tb: byte mismatch at index " & integer'image(i)
        severity error;
      if rd_bytes(i) /= FBODY(i) then
        any_fail := true;
      end if;
    end loop;

    -- pulse ack, confirm frame_ready deasserts
    wait until rising_edge(clk);
    ack <= '1';
    wait until rising_edge(clk);
    ack <= '0';
    wait for 500 ns;

    assert frame_ready = '0'
      report "eth_rx_tb: frame_ready did not clear after ack" severity error;
    if frame_ready /= '0' then
      any_fail := true;
    end if;

    -- ---------------------------------------------------------------
    -- Frame 2: send a second frame after ack, confirm it's received too.
    -- ---------------------------------------------------------------
    wait until frame_ready = '1' for 60 us;
    assert frame_ready = '1'
      report "eth_rx_tb: frame_ready never asserted (frame 2)" severity error;
    if frame_ready /= '1' then
      any_fail := true;
    end if;

    assert to_integer(rx_len) = FBODY'length
      report "eth_rx_tb: rx_len mismatch (frame 2)" severity error;
    if to_integer(rx_len) /= FBODY'length then
      any_fail := true;
    end if;

    for w in 0 to nwords - 1 loop
      rd_word_addr <= to_unsigned(w, 10);
      wait until rising_edge(clk);
      wait until rising_edge(clk);
      exp_word := rd_word;
      byte0 := exp_word(31 downto 24);
      byte1 := exp_word(23 downto 16);
      byte2 := exp_word(15 downto  8);
      byte3 := exp_word( 7 downto  0);
      idx := w * 4;
      if idx     <= FBODY'high then rd_bytes(idx)     := byte0; end if;
      if idx + 1 <= FBODY'high then rd_bytes(idx + 1) := byte1; end if;
      if idx + 2 <= FBODY'high then rd_bytes(idx + 2) := byte2; end if;
      if idx + 3 <= FBODY'high then rd_bytes(idx + 3) := byte3; end if;
    end loop;

    for i in FBODY'range loop
      assert rd_bytes(i) = FBODY(i)
        report "eth_rx_tb: byte mismatch (frame 2) at index " & integer'image(i)
        severity error;
      if rd_bytes(i) /= FBODY(i) then
        any_fail := true;
      end if;
    end loop;

    wait until rising_edge(clk);
    ack <= '1';
    wait until rising_edge(clk);
    ack <= '0';
    wait for 500 ns;

    assert frame_ready = '0'
      report "eth_rx_tb: frame_ready did not clear after ack (frame 2)" severity error;

    if not any_fail then
      report "eth_rx_tb PASSED" severity note;
    end if;

    sim_done <= true;
    wait;
  end process;

end architecture;
