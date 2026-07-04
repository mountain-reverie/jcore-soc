-- Focused readback tb for the eth_tx device's RX side: drives a Manchester
-- ARP-request frame into rx_in (same convention as eth_rx_tb), then reads the
-- frame back through the CPU-bus RX registers exactly the way rom/banner.c's
-- eth_recv() does -- RX_STATUS (0x900) poll, RX_LEN (0x904), a walk of
-- RX_DATA (0x908, auto-incrementing word pointer), then RX_ACK (0x90C) -- and
-- asserts the bytes read back are byte-exact. This isolates the auto-increment
-- read-pointer walk from the full SoC boot sim. Runs in microseconds.
--
-- Reads use the jcore bus convention (assert en, hold until the registered
-- ack, sample db_o.d on the ack cycle), which is exactly how the CPU's
-- eth_recv() walks the buffer. This is a regression guard for the eth_tx read
-- path (registered ack aligned with registered read data); an earlier version
-- of eth_tx paired a combinational ack with registered read data, which
-- shifted every read by one cycle and made eth_recv's frame-ready poll read the
-- wrong register.
library ieee;
use ieee.std_logic_1164.all;
use ieee.numeric_std.all;
use work.cpu2j0_pack.all;

entity eth_rx_readback_tb is
end entity;

architecture sim of eth_rx_readback_tb is
  constant CLK12    : time := 83333 ps;  -- ~12 MHz CPU bus clock
  constant PETH     : time := 25 ns;     -- 40 MHz clk_eth
  constant HALF_BIT : time := 50 ns;     -- 100 ns Manchester bit / 2

  signal clk     : std_logic := '0';
  signal clk_eth : std_logic := '0';
  signal rst     : std_logic := '1';
  signal db_i    : cpu_data_o_t := NULL_DATA_O;
  signal db_o    : cpu_data_i_t;
  signal mdi_p   : std_logic;
  signal mdi_n   : std_logic;
  signal tx_done : std_logic;
  signal rx_in   : std_logic := '0';
  signal rx_irq  : std_logic;

  signal sim_done : boolean := false;

  type byte_arr is array (natural range <>) of std_logic_vector(7 downto 0);

  -- ARP request fbody (dest..target PA) + FCS, 46 bytes after the SFD. Same
  -- content as icesugar_top_tb's REQ_FRAME (minus preamble/SFD).
  constant ABODY : byte_arr := (
    x"FF", x"FF", x"FF", x"FF", x"FF", x"FF",  -- eth dest broadcast
    x"AA", x"BB", x"CC", x"DD", x"EE", x"01",  -- eth src host MAC
    x"08", x"06",                              -- ethertype ARP
    x"00", x"01", x"08", x"00", x"06", x"04", x"00", x"01",
    x"AA", x"BB", x"CC", x"DD", x"EE", x"01",  -- sender HA
    x"C0", x"A8", x"01", x"63",                -- sender PA 192.168.1.99
    x"00", x"00", x"00", x"00", x"00", x"00",  -- target HA
    x"C0", x"A8", x"01", x"0A",                -- target PA 192.168.1.10
    x"C3", x"7A", x"F3", x"7F");               -- FCS (LE) 0x7FF37AC3

  constant PRE_SFD : byte_arr :=
    (x"55", x"55", x"55", x"55", x"55", x"55", x"55", x"D5");

  procedure send_manchester(signal rx : out std_logic; fbody : byte_arr) is
    variable b : std_logic;
  begin
    for k in PRE_SFD'range loop
      for i in 0 to 7 loop
        b := PRE_SFD(k)(i); rx <= not b; wait for HALF_BIT; rx <= b; wait for HALF_BIT;
      end loop;
    end loop;
    for k in fbody'range loop
      for i in 0 to 7 loop
        b := fbody(k)(i); rx <= not b; wait for HALF_BIT; rx <= b; wait for HALF_BIT;
      end loop;
    end loop;
    rx <= '0';
  end procedure;

begin

  dut: entity work.eth_tx
    port map (clk => clk, rst => rst, db_i => db_i, db_o => db_o,
              mdi_p => mdi_p, mdi_n => mdi_n, tx_done => tx_done,
              rx_in => rx_in, rx_irq => rx_irq, clk_eth => clk_eth);

  clk     <= not clk     after CLK12/2 when not sim_done else '0';
  clk_eth <= not clk_eth after PETH/2  when not sim_done else '0';

  stim: process
    -- gapped read: deassert en for a cycle between reads.
    procedure rd_gapped(addr : std_logic_vector(11 downto 0);
                        result : out std_logic_vector(31 downto 0)) is
    begin
      wait until rising_edge(clk);
      db_i.en <= '1'; db_i.rd <= '1'; db_i.wr <= '0'; db_i.a <= x"00000" & addr;
      wait until db_o.ack = '1';
      wait until rising_edge(clk);
      db_i.en <= '0'; db_i.rd <= '0';
      wait until rising_edge(clk);
      result := db_o.d;
    end procedure;

    procedure wr(addr : std_logic_vector(11 downto 0); data : std_logic_vector(31 downto 0)) is
    begin
      wait until rising_edge(clk);
      db_i.en <= '1'; db_i.wr <= '1'; db_i.rd <= '0'; db_i.we <= "1111";
      db_i.a <= x"00000" & addr; db_i.d <= data;
      wait until db_o.ack = '1';
      wait until rising_edge(clk);
      db_i.en <= '0'; db_i.wr <= '0'; db_i.we <= "0000";
    end procedure;

    variable rdw     : std_logic_vector(31 downto 0);
    variable rxb     : byte_arr(0 to ABODY'length-1);
    variable nbytes  : integer;
    variable any_bad : boolean := false;
  begin
    rst <= '1';
    for i in 0 to 8 loop wait until rising_edge(clk); end loop;
    rst <= '0';
    wait for 1 us;

    -- drive the ARP request into the RX PHY
    send_manchester(rx_in, ABODY);

    -- wait for frame_ready (RX_STATUS bit0)
    for tries in 0 to 200 loop
      rd_gapped(x"900", rdw);
      exit when rdw(0) = '1';
    end loop;
    assert rdw(0) = '1' report "eth_rx_readback_tb: frame_ready never set" severity failure;

    rd_gapped(x"904", rdw);
    nbytes := to_integer(unsigned(rdw(11 downto 0)));
    assert nbytes = ABODY'length
      report "eth_rx_readback_tb: RX_LEN got " & integer'image(nbytes)
             & " expected " & integer'image(ABODY'length) severity failure;

    -- walk RX_DATA exactly like eth_recv(): one gapped read per 32-bit word
    for w in 0 to (nbytes + 3) / 4 - 1 loop
      rd_gapped(x"908", rdw);
      rxb(w*4)                       := rdw(31 downto 24);
      if w*4+1 <= rxb'high then rxb(w*4+1) := rdw(23 downto 16); end if;
      if w*4+2 <= rxb'high then rxb(w*4+2) := rdw(15 downto  8); end if;
      if w*4+3 <= rxb'high then rxb(w*4+3) := rdw( 7 downto  0); end if;
    end loop;

    for i in ABODY'range loop
      if rxb(i) /= ABODY(i) then
        any_bad := true;
        report "eth_rx_readback_tb: byte mismatch at " & integer'image(i)
               & " got " & integer'image(to_integer(unsigned(rxb(i))))
               & " exp " & integer'image(to_integer(unsigned(ABODY(i))))
          severity error;
      end if;
    end loop;

    assert not any_bad
      report "eth_rx_readback_tb: RX readback CORRUPT (read-pointer walk bug)" severity failure;

    wr(x"90C", x"00000001");   -- RX_ACK: release + rewind

    report "eth_rx_readback_tb PASSED" severity note;
    sim_done <= true;
    wait;
  end process;

end architecture;
