-- eth_tx : 10BASE-T Manchester TX device.
--
-- CPU-bus (cpu2j0_pack) attached device running in the 12 MHz clk domain.
-- Instantiates an SB_PLL40_CORE (12->20 MHz, unbound at synth / behavioural in
-- sim via components/emac/sb_pll40_core_sim.vhd) to drive the 20 MHz clk_eth
-- domain, an inferred dual-clock frame buffer (32-bit write @clk / 8-bit read
-- @clk_eth, big-endian on the wire), and the eth_tx_phy Manchester serializer.
--
-- Register map (device-local byte offsets, decoded on db_i.a(11 downto 0)):
--   0x800 write     = TX_DATA : append 32-bit word to buffer, wr ptr += 4 bytes
--                              (big-endian: d(31:24) is byte 0 / lowest address)
--   0x804 write bit0= TX_RST_PTR : reset the write pointer to 0
--   0x808 write     = TX_LEN  : frame length in bytes
--   0x80C write bit0= TX_GO   : start transmission
--   0x810 read  bit0= busy    : 1 while a transmission is in progress
--
-- PLL params from `icepll -i 12 -o 20`:
--   DIVR=0 DIVF=52 DIVQ=5 FILTER_RANGE=1 (achieved 19.875 MHz, FEEDBACK SIMPLE)
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
    tx_done : out std_logic);
end entity;

architecture rtl of eth_tx is

  component SB_PLL40_CORE is
    generic (
      FEEDBACK_PATH : string := "SIMPLE";
      PLLOUT_SELECT : string := "GENCLK";
      DIVR          : std_logic_vector(3 downto 0) := "0000";
      DIVF          : std_logic_vector(6 downto 0) := "0000000";
      DIVQ          : std_logic_vector(2 downto 0) := "000";
      FILTER_RANGE  : std_logic_vector(2 downto 0) := "000");
    port (
      REFERENCECLK : in  std_logic;
      PLLOUTCORE   : out std_logic;
      PLLOUTGLOBAL : out std_logic;
      RESETB       : in  std_logic;
      BYPASS       : in  std_logic;
      LOCK         : out std_logic);
  end component;

  -- Address offsets
  constant A_TX_DATA : std_logic_vector(11 downto 0) := x"800";
  constant A_RST_PTR : std_logic_vector(11 downto 0) := x"804";
  constant A_TX_LEN  : std_logic_vector(11 downto 0) := x"808";
  constant A_TX_GO   : std_logic_vector(11 downto 0) := x"80C";
  constant A_BUSY    : std_logic_vector(11 downto 0) := x"810";

  -- PLL / clk_eth
  signal clk_eth   : std_logic;
  signal pll_lock  : std_logic;

  -- Dual-clock frame buffer (32-bit words)
  type mem_t is array (0 to BUF_WORDS-1) of std_logic_vector(31 downto 0);
  signal mem : mem_t := (others => (others => '0'));

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
  signal rd_word_r : std_logic_vector(31 downto 0) := (others => '0');
  signal rd_bsel_r : std_logic_vector(1 downto 0) := (others => '0');

begin

  ------------------------------------------------------------------
  -- PLL: 12 MHz -> 20 MHz
  ------------------------------------------------------------------
  pll: SB_PLL40_CORE
    generic map (
      FEEDBACK_PATH => "SIMPLE",
      PLLOUT_SELECT => "GENCLK",
      DIVR          => "0000",     -- 0
      DIVF          => "0110100",  -- 52
      DIVQ          => "101",      -- 5
      FILTER_RANGE  => "001")      -- 1
    port map (
      REFERENCECLK => clk,
      PLLOUTCORE   => open,
      PLLOUTGLOBAL => clk_eth,
      RESETB       => '1',
      BYPASS       => '0',
      LOCK         => pll_lock);

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

      if db_i.en = '1' and db_i.wr = '1' then
        case db_i.a(11 downto 0) is
          when A_TX_DATA =>
            mem(to_integer(wr_ptr(11 downto 2))) <= db_i.d;
            wr_ptr <= wr_ptr + 4;
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
          when others => null;
        end case;
      end if;

      if db_i.en = '1' and db_i.rd = '1' then
        case db_i.a(11 downto 0) is
          when A_BUSY =>
            rdata_r(0) <= busy_sync or go_pend;
          when others => null;
        end case;
      end if;

      if rst = '1' then
        wr_ptr    <= (others => '0');
        tx_len_r  <= (others => '0');
        go_tgl    <= '0';
        go_pend   <= '0';
        tx_done   <= '0';
        busy_meta <= '0';
        busy_sync <= '0';
        busy_prev <= '0';
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
      rd_word_r <= mem(to_integer(rd_addr(11 downto 2)));
      rd_bsel_r <= std_logic_vector(rd_addr(1 downto 0));
    end if;
  end process;

  -- big-endian byte mux: byte 0 (lowest addr) = d(31:24)
  with rd_bsel_r select
    rd_data <= rd_word_r(31 downto 24) when "00",
               rd_word_r(23 downto 16) when "01",
               rd_word_r(15 downto  8) when "10",
               rd_word_r( 7 downto  0) when others;

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

end architecture;
