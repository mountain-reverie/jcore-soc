-- eth_tx : 10BASE-T Manchester TX device.
--
-- CPU-bus (cpu2j0_pack) attached device running in the 12 MHz clk domain.
-- The 20 MHz clk_eth domain (dual-clock frame buffer: 32-bit write @clk /
-- 8-bit read @clk_eth, big-endian on the wire) is driven by a clk_eth input
-- port fed by the board clock generator (ice_clkgen, SB_PLL40_2_PAD PORTB) --
-- the UP5K has a single PLL bel, which must be shared with the 12 MHz CPU
-- clock passthrough, so the PLL can no longer live inside this device.
--
-- Register map (device-local byte offsets, decoded on db_i.a(11 downto 0)):
--   0x800 write     = TX_DATA : append 32-bit word to buffer, wr ptr += 4 bytes
--                              (big-endian: d(31:24) is byte 0 / lowest address)
--   0x804 write bit0= TX_RST_PTR : reset the write pointer to 0
--   0x808 write     = TX_LEN  : frame length in bytes
--   0x80C write bit0= TX_GO   : start transmission
--   0x810 read  bit0= busy    : 1 while a transmission is in progress
--
--   0x900 read  bit0= frame_ready, bit1= overrun : RX_STATUS
--   0x904 read       = rx frame length in bytes  : RX_LEN
--   0x908 read       = next 32-bit word of the received frame, big-endian;
--                       each read auto-increments the RX read-word pointer
--                       so successive reads walk the frame : RX_DATA
--   0x90C write bit0= pulse eth_rx.ack (release the frame buffer) and reset
--                      the RX read-word pointer to 0        : RX_ACK
library ieee;
use ieee.std_logic_1164.all;
use ieee.numeric_std.all;
use work.cpu2j0_pack.all;

entity eth_tx is
  generic (
    -- Buffer depth in 32-bit words (default 512 = 2 KiB).
    BUF_WORDS : integer := 512);
  port (
    clk     : in  std_logic;                 -- 12 MHz CPU-bus clock
    rst     : in  std_logic;
    db_i    : in  cpu_data_o_t;              -- from CPU
    db_o    : out cpu_data_i_t;              -- to CPU
    mdi_p   : out std_logic;
    mdi_n   : out std_logic;
    tx_done : out std_logic;
    rx_in   : in  std_logic;                 -- 10BASE-T RX MDI (LVDS pad input)
    rx_irq  : out std_logic;                 -- clk domain, 1-cycle pulse
    clk_eth : in  std_logic);                -- ~20 MHz PHY-domain clock
end entity;

architecture rtl of eth_tx is

  -- Address offsets
  constant A_TX_DATA : std_logic_vector(11 downto 0) := x"800";
  constant A_RST_PTR : std_logic_vector(11 downto 0) := x"804";
  constant A_TX_LEN  : std_logic_vector(11 downto 0) := x"808";
  constant A_TX_GO   : std_logic_vector(11 downto 0) := x"80C";
  constant A_BUSY    : std_logic_vector(11 downto 0) := x"810";

  constant A_RX_STATUS : std_logic_vector(11 downto 0) := x"900";
  constant A_RX_LEN    : std_logic_vector(11 downto 0) := x"904";
  constant A_RX_DATA   : std_logic_vector(11 downto 0) := x"908";
  constant A_RX_ACK    : std_logic_vector(11 downto 0) := x"90C";

  -- Dual-clock frame buffer, split into 4 byte-lane RAMs (one SB_RAM40 each).
  -- Lane i holds byte i of each 32-bit word (big-endian: lane0 = first byte
  -- on the wire, i.e. d(31:24)). Each lane is BUF_WORDS deep x 8 bits wide,
  -- with a clocked write port (@clk) and a clocked registered read port
  -- (@clk_eth) -- the symmetric-geometry shape yosys maps onto SB_RAM40.
  type lane_t is array (0 to BUF_WORDS-1) of std_logic_vector(7 downto 0);
  signal mem_lane0 : lane_t;
  signal mem_lane1 : lane_t;
  signal mem_lane2 : lane_t;
  signal mem_lane3 : lane_t;

  -- clk (12 MHz) domain register state
  signal wr_ptr    : unsigned(11 downto 0) := (others => '0');  -- byte pointer
  signal tx_len_r  : unsigned(11 downto 0) := (others => '0');
  signal go_tgl    : std_logic := '0';   -- toggles on each TX_GO
  signal go_pend   : std_logic := '0';   -- set at GO, cleared once phy busy seen
  signal busy_meta : std_logic := '0';
  signal busy_sync : std_logic := '0';
  signal busy_prev : std_logic := '0';
  signal rdata_r   : std_logic_vector(31 downto 0) := (others => '0');

  -- clk_eth (20 MHz) domain
  signal go_meta   : std_logic := '0';
  signal go_s      : std_logic := '0';
  signal go_prev   : std_logic := '0';
  signal tx_start  : std_logic := '0';
  signal tx_start_d: std_logic := '0';  -- tx_start delayed 1 cycle, feeds the phy
  signal tx_len_eth: unsigned(11 downto 0) := (others => '0');
  signal phy_busy  : std_logic;
  signal phy_done  : std_logic;
  signal rd_addr   : unsigned(11 downto 0);
  signal rd_data   : std_logic_vector(7 downto 0);
  type lane_q_t is array (0 to 3) of std_logic_vector(7 downto 0);
  signal rd_lane_q : lane_q_t := (others => (others => '0'));
  signal rd_bsel_r : std_logic_vector(1 downto 0) := (others => '0');

  -- clk (12 MHz) domain: eth_rx interface
  signal rx_word_ptr  : unsigned(9 downto 0) := (others => '0');  -- word pointer
  signal rx_rd_word   : std_logic_vector(31 downto 0);
  signal rx_frame_rdy : std_logic;
  signal rx_len       : unsigned(11 downto 0);
  signal rx_ack       : std_logic := '0';
  signal rx_overrun   : std_logic;

begin

  ------------------------------------------------------------------
  -- 12 MHz register / bus process + buffer write port
  ------------------------------------------------------------------
  reg_proc: process (clk) is
  begin
    if rising_edge(clk) then
      -- CDC in: phy busy (clk_eth) -> clk
      busy_meta <= phy_busy;
      busy_sync <= busy_meta;
      busy_prev <= busy_sync;

      tx_done <= '0';
      -- tx_done pulse on falling edge of synchronized busy
      if busy_prev = '1' and busy_sync = '0' then
        tx_done <= '1';
      end if;
      -- clear pending once the phy has actually started
      if busy_sync = '1' then
        go_pend <= '0';
      end if;

      -- default bus response
      rdata_r <= (others => '0');

      -- The TX_DATA memory write is pulled out of the register-map case
      -- statement below and given its own plain `if` (rather than being a
      -- branch of the case): yosys (via ghdl-yosys-plugin) only recognizes
      -- the array-write-in-a-clocked-process pattern as inferable memory
      -- when it is guarded by a simple `if`, not when it sits inside a
      -- `case`/`when` -- wrapping the write in `case db_i.a(...) is ...`
      -- causes the whole 2 KiB buffer to be expanded into flip-flops
      -- instead of mapping onto SB_RAM40.
      if db_i.en = '1' and db_i.wr = '1' and db_i.a(11 downto 0) = A_TX_DATA then
        mem_lane0(to_integer(wr_ptr(11 downto 2))) <= db_i.d(31 downto 24);
        mem_lane1(to_integer(wr_ptr(11 downto 2))) <= db_i.d(23 downto 16);
        mem_lane2(to_integer(wr_ptr(11 downto 2))) <= db_i.d(15 downto  8);
        mem_lane3(to_integer(wr_ptr(11 downto 2))) <= db_i.d( 7 downto  0);
        wr_ptr <= wr_ptr + 4;
      end if;

      -- RX_ACK is a 1-cycle pulse into eth_rx.ack; default low each cycle.
      rx_ack <= '0';

      if db_i.en = '1' and db_i.wr = '1' then
        case db_i.a(11 downto 0) is
          when A_RST_PTR =>
            if db_i.d(0) = '1' then
              wr_ptr <= (others => '0');
            end if;
          when A_TX_LEN =>
            tx_len_r <= unsigned(db_i.d(11 downto 0));
          when A_TX_GO =>
            if db_i.d(0) = '1' then
              go_tgl  <= not go_tgl;
              go_pend <= '1';
            end if;
          when A_RX_ACK =>
            if db_i.d(0) = '1' then
              rx_ack      <= '1';
              rx_word_ptr <= (others => '0');
            end if;
          when others => null;
        end case;
      end if;

      if db_i.en = '1' and db_i.rd = '1' then
        case db_i.a(11 downto 0) is
          when A_BUSY =>
            rdata_r(0) <= busy_sync or go_pend;
          when A_RX_STATUS =>
            rdata_r(0) <= rx_frame_rdy;
            rdata_r(1) <= rx_overrun;
          when A_RX_LEN =>
            rdata_r(11 downto 0) <= std_logic_vector(rx_len);
          when A_RX_DATA =>
            -- eth_rx buffer read is registered (1-cycle latency behind
            -- rd_word_addr, mirrored from the TX-side SB_RAM40 read port
            -- above): rx_rd_word reflects rx_word_ptr as it was presented
            -- on the previous cycle. Advancing the pointer here (same
            -- cycle as the read) presents the next word one cycle ahead of
            -- when software's *next* A_RX_DATA read will land, so back-to-
            -- back reads with no intervening bus cycle can observe one
            -- cycle of latency skew; exercised/confirmed in the Task-7 sim.
            rdata_r <= rx_rd_word;
            rx_word_ptr <= rx_word_ptr + 1;
          when others => null;
        end case;
      end if;

      if rst = '1' then
        wr_ptr      <= (others => '0');
        tx_len_r    <= (others => '0');
        go_tgl      <= '0';
        go_pend     <= '0';
        tx_done     <= '0';
        busy_meta   <= '0';
        busy_sync   <= '0';
        busy_prev   <= '0';
        rx_word_ptr <= (others => '0');
        rx_ack      <= '0';
      end if;
    end if;
  end process;

  -- Combinational ack (mirrors uart.vhd / pio.vhd / spi2.vhd / gpio2.vhd
  -- convention): the device never stalls the bus.
  db_o.ack <= db_i.en;
  db_o.d   <= rdata_r;

  ------------------------------------------------------------------
  -- 20 MHz domain: buffer read port (byte, big-endian) + GO CDC
  ------------------------------------------------------------------
  eth_proc: process (clk_eth) is
  begin
    if rising_edge(clk_eth) then
      -- CDC in: go toggle (clk) -> clk_eth, edge detect -> 1-cycle pulse
      go_meta <= go_tgl;
      go_s    <= go_meta;
      go_prev <= go_s;
      tx_start <= go_s xor go_prev;

      -- tx_len_r (clk domain) is latched into the clk_eth domain right here,
      -- on the synchronized tx_start pulse, instead of being read directly
      -- by the phy (which would be an unsynchronized multi-bit CDC). This
      -- relies on software ordering: TX_LEN is written before TX_GO, and the
      -- GO toggle-sync above already adds a couple of clk_eth cycles of
      -- margin, so tx_len_r is guaranteed stable by the time tx_start pulses.
      if tx_start = '1' then
        tx_len_eth <= tx_len_r;
      end if;
      -- tx_start_d is tx_start delayed one more clk_eth cycle: it feeds the
      -- phy so tx_len_eth (latched above, valid only from the cycle *after*
      -- tx_start) is already stable by the time the phy consumes it.
      tx_start_d <= tx_start;

      if rst = '1' then
        go_meta    <= '0';
        go_s       <= '0';
        go_prev    <= '0';
        tx_start   <= '0';
        tx_start_d <= '0';
        tx_len_eth <= (others => '0');
      end if;
    end if;
  end process;

  -- Registered read (models the real EBR: rd_data valid one clk_eth cycle
  -- after rd_addr is presented), written as a clocked process so synthesis
  -- (yosys synth_ice40) prefers SB_RAM40 inference over distributed LUT RAM.
  -- The eth_tx_phy prefetches accordingly (see eth_tx_phy.vhd).
  ram_rd_proc: process (clk_eth) is
  begin
    if rising_edge(clk_eth) then
      rd_lane_q(0) <= mem_lane0(to_integer(rd_addr(11 downto 2)));
      rd_lane_q(1) <= mem_lane1(to_integer(rd_addr(11 downto 2)));
      rd_lane_q(2) <= mem_lane2(to_integer(rd_addr(11 downto 2)));
      rd_lane_q(3) <= mem_lane3(to_integer(rd_addr(11 downto 2)));
      -- lane select registered alongside the data so the mux below uses the
      -- select value matching the 1-cycle-delayed lane data.
      rd_bsel_r <= std_logic_vector(rd_addr(1 downto 0));
    end if;
  end process;

  -- big-endian byte mux: byte 0 (lowest addr) = lane0
  with rd_bsel_r select
    rd_data <= rd_lane_q(0) when "00",
               rd_lane_q(1) when "01",
               rd_lane_q(2) when "10",
               rd_lane_q(3) when others;

  ------------------------------------------------------------------
  -- Manchester serializer PHY (20 MHz)
  ------------------------------------------------------------------
  phy: entity work.eth_tx_phy
    port map (
      clk_eth  => clk_eth,
      rst      => rst,
      tx_start => tx_start_d,
      tx_len   => tx_len_eth,
      rd_addr  => rd_addr,
      rd_data  => rd_data,
      busy     => phy_busy,
      done     => phy_done,
      mdi_p    => mdi_p,
      mdi_n    => mdi_n);

  ------------------------------------------------------------------
  -- 10BASE-T RX framer/buffer/CDC (clk / clk_eth)
  ------------------------------------------------------------------
  rx: entity work.eth_rx
    port map (
      clk          => clk,
      clk_eth      => clk_eth,
      rst          => rst,
      rx_in        => rx_in,
      rd_word_addr => rx_word_ptr,
      rd_word      => rx_rd_word,
      frame_ready  => rx_frame_rdy,
      rx_len       => rx_len,
      ack          => rx_ack,
      overrun      => rx_overrun,
      rx_irq       => rx_irq);

end architecture;
