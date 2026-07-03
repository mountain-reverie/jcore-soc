-- eth_rx : 10BASE-T Manchester RX framer + byte-lane frame buffer + CDC.
--
-- Consumes eth_rx_phy (Task 3): recovered bits (bit_o/bit_valid, LSB-first
-- within byte) and carrier (line activity) in the 40 MHz clk_eth domain.
--
-- Framer FSM (clk_eth): shifts recovered bits into a byte assembler, looks
-- for the SFD byte (0xD5) to establish byte alignment (any earlier
-- preamble bits are simply discarded -- the SFD byte pattern itself can
-- never appear misaligned within a run of alternating 0x55 preamble bits,
-- because 0xD5 = "11010101" has a bit at position 1 that breaks the
-- 0/1-alternating pattern). Once aligned, subsequent assembled bytes are
-- written into the byte-lane RX buffer, byte 0 = first byte after the SFD
-- (i.e. destination MAC byte 0). On carrier de-assertion, the byte count
-- is latched and a frame-complete flag is raised (subject to the
-- overrun/CDC handshake below).
--
-- Byte-lane RX buffer: 4x 8-bit RAMs, one SB_RAM40 each (same shape as
-- eth_tx.vhd's TX buffer): byte i -> lane (i mod 4), word addr (i/4),
-- write @clk_eth via a plain (uncased) `if`. Read: 32-bit @clk, registered,
-- muxing the 4 lanes by rd_word_addr. Depth BUF_WORDS (default 512 = 2KB).
-- Big-endian: first wire byte appears in rd_word(31 downto 24).
--
-- CDC: a clk_eth-domain frame-complete toggle is 2-FF synchronized into
-- the clk domain to produce frame_ready (level, held until ack). rx_len is
-- latched (clk_eth domain) before the toggle flips, so it is stable by the
-- time frame_ready asserts in the clk domain. ack (clk, 1-cycle pulse) is
-- itself toggle/2-FF synchronized back into clk_eth to clear/re-arm the
-- flag. If a new frame completes in clk_eth while the previous frame_ready
-- is still pending (un-acked), the new frame is dropped (buffer/count not
-- overwritten) and overrun is latched (sticky until reset). rx_irq pulses
-- one clk cycle each time frame_ready transitions 0->1.
library ieee;
use ieee.std_logic_1164.all;
use ieee.numeric_std.all;

entity eth_rx is
  generic (
    -- Buffer depth in 32-bit words (default 512 = 2 KiB = 2048 bytes).
    BUF_WORDS : integer := 512);
  port (
    clk          : in  std_logic;                     -- 12 MHz CPU-bus clock
    clk_eth      : in  std_logic;                     -- 40 MHz PHY-domain clock
    rst          : in  std_logic;
    rx_in        : in  std_logic;
    rd_word_addr : in  unsigned(9 downto 0);
    rd_word      : out std_logic_vector(31 downto 0);
    frame_ready  : out std_logic;                      -- clk domain, level
    rx_len       : out unsigned(11 downto 0);          -- clk domain, byte count
    ack          : in  std_logic;                      -- clk domain, 1-cycle pulse
    overrun      : out std_logic;
    rx_irq       : out std_logic);                     -- clk domain, 1-cycle pulse
end entity;

architecture rtl of eth_rx is

  -- SFD byte, LSB-first bit order as delivered by eth_rx_phy: bit0..bit7 =
  -- 1,0,1,0,1,0,1,1  =>  0xD5 = "11010101" (bit7..bit0).
  constant SFD_BYTE : std_logic_vector(7 downto 0) := x"D5";

  constant BYTE_ADDR_BITS : integer := 11; -- byte counter width (up to 2048 bytes)
  constant CNT_BITS       : integer := 16; -- byte_cnt width: wide enough that it
                                            -- never wraps back into the valid
                                            -- [0, BUF_WORDS*4) range for any
                                            -- realistic (even jabber-length) frame

  -- ---------------------------------------------------------------------
  -- eth_rx_phy instance
  -- ---------------------------------------------------------------------
  signal bit_o     : std_logic;
  signal bit_valid : std_logic;
  signal carrier   : std_logic;
  signal carrier_prev : std_logic := '0';

  -- ---------------------------------------------------------------------
  -- Framer FSM (clk_eth)
  -- ---------------------------------------------------------------------
  type fsm_t is (S_IDLE, S_ALIGN, S_DATA);
  signal state       : fsm_t := S_IDLE;
  signal shreg       : std_logic_vector(7 downto 0) := (others => '0');
  signal bit_cnt     : unsigned(2 downto 0) := (others => '0'); -- 0..7 within current byte
  signal byte_cnt     : unsigned(CNT_BITS-1 downto 0) := (others => '0'); -- bytes seen since SFD
                                                                            -- (wide: see CNT_BITS)

  signal frame_done_tgl  : std_logic := '0'; -- toggles on each completed frame (clk_eth)
  signal frame_len_eth   : unsigned(11 downto 0) := (others => '0');
  signal overrun_r       : std_logic := '0';

  -- Per-frame accept/drop decision, latched once at frame start (S_ALIGN ->
  -- S_DATA, i.e. on SFD match) from the pending_ack value at that instant.
  -- Sampling once (instead of re-checking pending_ack every byte) avoids the
  -- mid-frame-ack race where an ack crossing the CDC partway through a frame
  -- would otherwise splice together dropped-then-accepted bytes of the same
  -- frame. A frame is thus either written in full or dropped in full.
  signal frame_drop      : std_logic := '0';

  -- ---------------------------------------------------------------------
  -- Byte-lane RX buffer, 4x SB_RAM40-shaped arrays
  -- ---------------------------------------------------------------------
  type lane_t is array (0 to BUF_WORDS-1) of std_logic_vector(7 downto 0);
  signal mem_lane0 : lane_t;
  signal mem_lane1 : lane_t;
  signal mem_lane2 : lane_t;
  signal mem_lane3 : lane_t;

  signal pending_ack : std_logic := '0'; -- '1' while a completed frame awaits ack (clk_eth view)

  -- Shared combinational write-strobe/data/index terms feeding the four
  -- per-lane write processes (see ram_wr_laneN below).
  signal wr_en       : std_logic := '0';
  signal wr_byte     : std_logic_vector(7 downto 0);
  signal wr_word_idx : integer := 0;
  signal wr_lane_idx : integer := 0;

  -- clk domain: buffer read port
  type lane_q_t is array (0 to 3) of std_logic_vector(7 downto 0);
  signal rd_lane_q : lane_q_t := (others => (others => '0'));

  -- ---------------------------------------------------------------------
  -- CDC: clk_eth -> clk (frame_done_tgl -> frame_ready)
  -- ---------------------------------------------------------------------
  signal fr_meta, fr_s, fr_prev : std_logic := '0';
  signal frame_ready_r : std_logic := '0';
  signal rx_len_r      : unsigned(11 downto 0) := (others => '0');
  signal rx_irq_r      : std_logic := '0';

  -- ---------------------------------------------------------------------
  -- CDC: clk -> clk_eth (ack -> ack_tgl -> clear/re-arm)
  -- ---------------------------------------------------------------------
  signal ack_tgl  : std_logic := '0'; -- clk domain, toggles on each ack pulse
  signal ack_meta, ack_s : std_logic := '0'; -- clk_eth domain synchronizer

begin

  ------------------------------------------------------------------
  -- PHY instance
  ------------------------------------------------------------------
  phy: entity work.eth_rx_phy
    port map (
      clk_eth   => clk_eth,
      rst       => rst,
      rx_in     => rx_in,
      bit_o     => bit_o,
      bit_valid => bit_valid,
      carrier   => carrier);

  ------------------------------------------------------------------
  -- clk_eth domain: ack toggle synchronizer (clk -> clk_eth) and
  -- "pending_ack" tracking (has the current frame_done_tgl value been
  -- acked yet?)
  ------------------------------------------------------------------
  -- pending_ack is '1' from the cycle a frame completes (frame_done_tgl
  -- flips) until the synchronized ack toggle catches up (ack_s = frame_done_tgl).
  pending_ack <= '1' when ack_s /= frame_done_tgl else '0';

  ------------------------------------------------------------------
  -- Framer FSM + buffer write (clk_eth)
  ------------------------------------------------------------------
  framer_proc: process (clk_eth) is
  begin
    if rising_edge(clk_eth) then
      -- 2-FF sync of the clk-domain ack toggle
      ack_meta <= ack_tgl;
      ack_s    <= ack_meta;

      carrier_prev <= carrier;

      case state is
        when S_IDLE =>
          if carrier = '1' then
            state   <= S_ALIGN;
            shreg   <= (others => '0');
            bit_cnt <= (others => '0');
          end if;

        when S_ALIGN =>
          if carrier = '0' then
            state <= S_IDLE;
          elsif bit_valid = '1' then
            -- Byte alignment is unknown at this point (carrier can assert
            -- on any bit boundary within the preamble), so the SFD search
            -- is a *sliding* 8-bit window check on every bit, not a
            -- fixed every-8th-bit check: shift in the new bit and compare
            -- the resulting window against SFD_BYTE unconditionally. Once
            -- found, that bit position becomes the locked byte boundary
            -- for the rest of the frame (bit_cnt reset to 0 in S_DATA).
            shreg <= bit_o & shreg(7 downto 1); -- LSB-first: newest bit -> MSB, shift right
            if (bit_o & shreg(7 downto 1)) = SFD_BYTE then
              state      <= S_DATA;
              bit_cnt    <= (others => '0');
              byte_cnt   <= (others => '0');
              -- Latch the accept/drop decision for this frame once, here,
              -- at frame start -- see frame_drop declaration above. This is
              -- immune to pending_ack changing (via a mid-frame ack
              -- crossing the CDC) later in the same frame.
              frame_drop <= pending_ack;
            end if;
          end if;

        when S_DATA =>
          if carrier = '0' then
            -- end of frame: latch length, raise frame-complete (unless a
            -- previous completed frame is still un-acked -> overrun).
            if frame_drop = '1' then
              overrun_r <= '1';
            else
              -- Saturate at 12 bits: byte_cnt (now 16 bits, see CNT_BITS)
              -- can in principle exceed 4095 for a pathological/jabber
              -- frame that keeps carrier asserted well past the buffer end;
              -- report the field's max value rather than silently
              -- truncating/wrapping frame_len_eth in that case.
              if byte_cnt > to_unsigned(4095, byte_cnt'length) then
                frame_len_eth <= (others => '1');
              else
                frame_len_eth <= resize(byte_cnt, 12);
              end if;
              frame_done_tgl <= not frame_done_tgl;
            end if;
            state <= S_IDLE;
          elsif bit_valid = '1' then
            shreg <= bit_o & shreg(7 downto 1);
            if bit_cnt = to_unsigned(7, 3) then
              bit_cnt <= (others => '0');
              -- a full byte (bit_o & shreg(7 downto 1)) is available this
              -- cycle; the actual RAM write is done in the unconditional
              -- `if` block below (plain-if pattern required for SB_RAM40
              -- inference -- see eth_tx.vhd comment).
              byte_cnt <= byte_cnt + 1;
            else
              bit_cnt <= bit_cnt + 1;
            end if;
          end if;

        when others =>
          state <= S_IDLE;
      end case;

      if rst = '1' then
        state          <= S_IDLE;
        shreg          <= (others => '0');
        bit_cnt        <= (others => '0');
        byte_cnt       <= (others => '0');
        frame_done_tgl <= '0';
        frame_len_eth  <= (others => '0');
        overrun_r      <= '0';
        frame_drop     <= '0';
        ack_meta       <= '0';
        ack_s          <= '0';
      end if;
    end if;
  end process;

  overrun <= overrun_r;

  -- Buffer write: kept as a single, plain (uncased) `if`, exactly matching
  -- the pattern eth_tx.vhd found necessary for SB_RAM40 inference -- a
  -- write guarded by a `case`/`when` gets expanded to flip-flops by yosys
  -- instead of mapping onto a block RAM.
  --
  -- Writes only proceed when: in S_DATA, a full byte has just been
  -- assembled (bit_cnt about to wrap from 7 to 0 while bit_valid='1'),
  -- there is room left in the buffer, and the current frame is not being
  -- dropped for overrun. The drop decision (frame_drop) is latched ONCE per
  -- frame at SFD-match time (see frame_drop declaration/assignment above),
  -- not re-evaluated per byte -- so a frame is always either written in
  -- full or dropped in full, even if the previous frame gets acked
  -- (clearing pending_ack) partway through this frame's reception.
  -- Common per-byte write-enable + word index, shared by the four
  -- per-lane write processes below. Each lane gets its OWN plain
  -- (case-free) `if rising_edge... if <cond> then mem(idx) <= data; end
  -- if;` process -- splitting the case/when lane-select out of the
  -- clocked write entirely -- because (as with eth_tx.vhd) yosys only
  -- infers SB_RAM40 for a write guarded by a simple `if`, not one behind
  -- a `case`/`when` (a case wrapping the array write there caused the
  -- whole buffer to expand into flip-flops instead of block RAM; the
  -- same applies to selecting *which* memory to write via a case).
  wr_en <= '1' when (state = S_DATA and bit_valid = '1' and bit_cnt = to_unsigned(7, 3)
                      and frame_drop = '0' and to_integer(byte_cnt) < BUF_WORDS * 4)
               else '0';
  wr_byte <= bit_o & shreg(7 downto 1);
  wr_word_idx <= to_integer(byte_cnt) / 4;
  wr_lane_idx <= to_integer(byte_cnt) mod 4;

  ram_wr_lane0: process (clk_eth) is
  begin
    if rising_edge(clk_eth) then
      if wr_en = '1' and wr_lane_idx = 0 then
        mem_lane0(wr_word_idx) <= wr_byte;
      end if;
    end if;
  end process;

  ram_wr_lane1: process (clk_eth) is
  begin
    if rising_edge(clk_eth) then
      if wr_en = '1' and wr_lane_idx = 1 then
        mem_lane1(wr_word_idx) <= wr_byte;
      end if;
    end if;
  end process;

  ram_wr_lane2: process (clk_eth) is
  begin
    if rising_edge(clk_eth) then
      if wr_en = '1' and wr_lane_idx = 2 then
        mem_lane2(wr_word_idx) <= wr_byte;
      end if;
    end if;
  end process;

  ram_wr_lane3: process (clk_eth) is
  begin
    if rising_edge(clk_eth) then
      if wr_en = '1' and wr_lane_idx = 3 then
        mem_lane3(wr_word_idx) <= wr_byte;
      end if;
    end if;
  end process;

  ------------------------------------------------------------------
  -- clk domain: buffer read port (registered, 32-bit, big-endian mux)
  ------------------------------------------------------------------
  ram_rd_proc: process (clk) is
  begin
    if rising_edge(clk) then
      rd_lane_q(0) <= mem_lane0(to_integer(rd_word_addr));
      rd_lane_q(1) <= mem_lane1(to_integer(rd_word_addr));
      rd_lane_q(2) <= mem_lane2(to_integer(rd_word_addr));
      rd_lane_q(3) <= mem_lane3(to_integer(rd_word_addr));
    end if;
  end process;

  rd_word <= rd_lane_q(0) & rd_lane_q(1) & rd_lane_q(2) & rd_lane_q(3);

  ------------------------------------------------------------------
  -- clk domain: frame_done_tgl -> frame_ready CDC, rx_len latch, ack ->
  -- ack_tgl, rx_irq pulse
  ------------------------------------------------------------------
  cdc_proc: process (clk) is
  begin
    if rising_edge(clk) then
      -- 2-FF sync of the clk_eth-domain frame-complete toggle
      fr_meta <= frame_done_tgl;
      fr_s    <= fr_meta;
      fr_prev <= fr_s;

      rx_irq_r <= '0';

      if fr_s /= fr_prev and frame_ready_r = '0' then
        -- newly synchronized frame-complete edge: latch len (already
        -- stable in the clk_eth domain by construction: frame_len_eth was
        -- written before frame_done_tgl toggled, and the toggle-sync above
        -- guarantees at least 2 clk_eth cycles + resync latency margin)
        -- and raise frame_ready.
        rx_len_r      <= frame_len_eth;
        frame_ready_r <= '1';
        rx_irq_r      <= '1';
      end if;

      if ack = '1' then
        ack_tgl       <= not ack_tgl;
        frame_ready_r <= '0';
      end if;

      if rst = '1' then
        fr_meta       <= '0';
        fr_s          <= '0';
        fr_prev       <= '0';
        frame_ready_r <= '0';
        rx_len_r      <= (others => '0');
        rx_irq_r      <= '0';
        ack_tgl       <= '0';
      end if;
    end if;
  end process;

  frame_ready <= frame_ready_r;
  rx_len      <= rx_len_r;
  rx_irq      <= rx_irq_r;

end architecture;
