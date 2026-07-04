library ieee;
use ieee.std_logic_1164.all;
use ieee.numeric_std.all;

entity eth_rx_phy is
  port (
    clk_eth   : in  std_logic;                    -- 40 MHz
    rst       : in  std_logic;
    rx_in     : in  std_logic;                     -- sliced LVDS RX (== mdi_p on the wire)
    bit_o     : out std_logic;
    bit_valid : out std_logic;                     -- 1-cycle strobe per recovered bit
    carrier   : out std_logic);
end entity;

-- 10BASE-T RX PHY: input sync -> squelch/carrier-detect -> 4x
-- oversample-and-vote (edge-locked) clock recovery -> Manchester bit decode.
--
-- (This block previously used an 8-bit DPLL/NCO for clock recovery; it was
-- replaced with the smaller edge-locked oversampler below to close a ~91 LC
-- UP5K fit overage. Same entity ports, same Manchester convention, and it
-- still passes eth_rx_phy_tb -- including the injected-jitter frame -- plus
-- the end-to-end icesugar_top_tb ARP loopback.)
--
-- Manchester convention (MUST match components/emac/eth_tx_phy.vhd SEND
-- state exactly -- confirmed by reading that file):
--   mdi_p <= diff(1); first half-bit:  diff <= (not b) & b   -> mdi_p = not b
--                      second half-bit: diff <= b & (not b)  -> mdi_p = b
--   i.e. every bit has a transition at the bit midpoint, and the recovered
--   bit value b is whatever the line reads during the SECOND half of the
--   bit period. rx_in is the direct LVDS slice of mdi_p, so:
--     first half-bit:  rx_in = not b
--     second half-bit: rx_in = b
--   Bytes are shifted out LSB-first by TX, so RX reassembly (done in a
--   later task) must also treat the first recovered bit of a byte as bit0.
architecture rtl of eth_rx_phy is

  -- ---------------------------------------------------------------------
  -- Input synchronizer + edge detect
  -- ---------------------------------------------------------------------
  signal rx_m1, rx_s, rx_prev : std_logic := '0';

  -- ---------------------------------------------------------------------
  -- Squelch / carrier detect
  -- ---------------------------------------------------------------------
  -- clk_eth = 40 MHz -> 25 ns/cycle. A locked Manchester bit stream always
  -- has an edge at least once per bit period (100 ns = 4 cycles, the
  -- guaranteed mid-bit transition), so a gap of > ~2 bit periods with no
  -- edge at all means the line has gone idle/quiet -- drop carrier.
  -- NO_EDGE_TIMEOUT = 8 cycles = 200 ns (2 bit periods) of silence.
  constant NO_EDGE_TIMEOUT : integer := 8;
  -- Require this many edges seen back-to-back (each within the timeout
  -- window of the previous one) before declaring carrier present -- a
  -- lone NLP link pulse is a single isolated transition and can never
  -- satisfy this, only a sustained ~5-10 MHz transition train (preamble
  -- or real data) can.
  constant LOCK_EDGES      : integer := 6;
  signal no_edge_cnt : unsigned(3 downto 0) := (others => '0');
  signal edge_run     : unsigned(3 downto 0) := (others => '0');
  signal carrier_r     : std_logic := '0';

  -- ---------------------------------------------------------------------
  -- 4x oversample-and-vote (edge-locked) clock recovery
  --   -- clearly delimited fit-fallback replacement for the former DPLL/NCO.
  -- ---------------------------------------------------------------------
  -- At 40 MHz, one 100 ns Manchester bit period is exactly 4 clk_eth cycles;
  -- each bit has a GUARANTEED transition at its midpoint (the TX convention
  -- above: first half-bit = not b, second half-bit = b). Manchester structure
  -- also means the recovered bit value is simply the line level in the SECOND
  -- half of the bit, i.e. the level *after* that mid-bit transition.
  --
  -- Lock strategy (no phase accumulator, no arithmetic): sample the
  -- synchronized line every clk_eth cycle and treat each accepted mid-bit
  -- edge as "here is a new bit; its value b is the new (post-edge) level".
  -- To tell mid-bit edges from bit-*boundary* edges (which appear a half-bit
  -- = 2 cycles after a mid-bit edge, only when adjacent bits are equal), a
  -- refractory counter blocks all edges for REFRACTORY cycles after each
  -- accepted edge: 2 < REFRACTORY < 4 rejects the +2-cycle boundary edge but
  -- re-arms in time for the next +4-cycle mid-bit edge. This is exact for
  -- clean Manchester and tolerates several ns of edge jitter (cycle-granular).
  -- Initial lock always latches onto a mid-bit edge because the 7-byte
  -- alternating preamble (0x55) contains ONLY mid-bit edges (its equal-half
  -- boundaries produce no transition), so the first accepted edge is a
  -- mid-bit edge and the rhythm is established before the SFD/data arrive.
  -- The 2-FF synchronizer's fixed 2-cycle delay shifts every edge equally, so
  -- it does not affect this edge-relative scheme.
  constant REFRACTORY : integer := 3;
  signal refr        : unsigned(2 downto 0) := (others => '0');

  signal bit_o_r     : std_logic := '0';
  signal bit_valid_r : std_logic := '0';

begin

  carrier   <= carrier_r;
  bit_o     <= bit_o_r;
  bit_valid <= bit_valid_r;

  process (clk_eth) is
  begin
    if rising_edge(clk_eth) then
      bit_valid_r <= '0';

      -- 2-FF synchronizer
      rx_m1 <= rx_in;
      rx_s  <= rx_m1;
      rx_prev <= rx_s;

      -- -----------------------------------------------------------------
      -- Squelch / carrier-detect
      -- -----------------------------------------------------------------
      if (rx_s xor rx_prev) = '1' then
        no_edge_cnt <= (others => '0');
        if edge_run /= to_unsigned(15, 4) then
          edge_run <= edge_run + 1;
        end if;
        if to_integer(edge_run) + 1 >= LOCK_EDGES then
          carrier_r <= '1';
        end if;
      else
        if no_edge_cnt = to_unsigned(NO_EDGE_TIMEOUT, 4) then
          carrier_r  <= '0';
          edge_run   <= (others => '0');
        else
          no_edge_cnt <= no_edge_cnt + 1;
        end if;
      end if;

      -- -----------------------------------------------------------------
      -- 4x oversample-and-vote (edge-locked) bit recovery
      -- -----------------------------------------------------------------
      -- refractory counter: counts down after each accepted edge, blocking
      -- edges (the +2-cycle boundary transition) until it reaches 0.
      if refr /= to_unsigned(0, refr'length) then
        refr <= refr - 1;
      end if;

      if (rx_s xor rx_prev) = '1' and refr = to_unsigned(0, refr'length) then
        -- Accepted mid-bit transition: the new (post-edge) line level IS the
        -- recovered bit value b (second-half level). Emit it and start the
        -- refractory window so the following half-bit boundary edge (if any)
        -- is ignored, while the next mid-bit edge one bit-period later is not.
        refr <= to_unsigned(REFRACTORY, refr'length);
        if carrier_r = '1' then
          bit_o_r     <= rx_s;
          bit_valid_r <= '1';
        end if;
      end if;

      if rst = '1' then
        rx_m1 <= '0'; rx_s <= '0'; rx_prev <= '0';
        no_edge_cnt <= (others => '0');
        edge_run    <= (others => '0');
        carrier_r   <= '0';
        refr        <= (others => '0');
        bit_o_r     <= '0';
        bit_valid_r <= '0';
      end if;
    end if;
  end process;

end architecture;
