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

-- 10BASE-T RX PHY: input sync -> squelch/carrier-detect -> DPLL clock
-- recovery -> Manchester bit decode.
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
  -- DPLL / NCO clock recovery
  --   -- keep this block clearly delimited: a later task (fit fallback)
  --   -- may replace it with a plain 4x oversampler if resources are tight.
  -- ---------------------------------------------------------------------
  -- N = 8-bit phase accumulator. At 40 MHz, 4 clk_eth cycles == one 100 ns
  -- bit period, so the nominal free-running increment is STEP = 2^8/4 = 64
  -- (accumulator wraps exactly once per bit with no jitter correction).
  constant N       : integer := 8;
  constant STEP    : unsigned(N-1 downto 0) := to_unsigned(64, N);
  -- TARGET = 128 = the phase value expected to coincide with the
  -- guaranteed Manchester mid-bit transition (the point exactly halfway
  -- through the bit period, where every bit -- regardless of its value --
  -- has an edge).
  constant TARGET  : integer := 128;
  -- Only correct on edges that land within +-64 of TARGET (i.e. phase in
  -- (64,192)); edges near phase 0/256 are bit-*boundary* transitions
  -- (only present when consecutive bits differ) and must NOT be used to
  -- steer the loop, or it will mis-lock a quarter-bit off.
  constant WINDOW  : integer := 64;
  -- Bang-bang correction step: pulls phase a fraction of the way to
  -- TARGET on every qualifying edge, averaging out several ns of jitter
  -- over the 7-byte preamble before data/SFD arrives.
  constant NUDGE   : unsigned(N-1 downto 0) := to_unsigned(24, N);
  -- Sample the line for the recovered bit value at phase >= SAMPLE_PT,
  -- i.e. the mid-point of the SECOND half-bit (3/4 through the bit
  -- period) -- exactly where the TX convention above says b is stable.
  constant SAMPLE_PT : integer := 192;
  -- 2-FF synchronizer latency (2 clk_eth cycles = 50 ns = half a 100 ns
  -- bit) expressed in phase units (128 = half of 256) -- see comment at
  -- point of use below.
  constant DELAY_COMP : unsigned(N-1 downto 0) := to_unsigned(128, N);

  signal phase       : unsigned(N-1 downto 0) := (others => '0');
  signal phase_prev  : unsigned(N-1 downto 0) := (others => '0');
  signal sampled     : std_logic := '0';       -- guards one capture per bit period
  signal bit_sample  : std_logic := '0';

  signal bit_o_r     : std_logic := '0';
  signal bit_valid_r : std_logic := '0';

begin

  carrier   <= carrier_r;
  bit_o     <= bit_o_r;
  bit_valid <= bit_valid_r;

  process (clk_eth) is
    variable p_int : integer range 0 to 2**N - 1;
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
      -- DPLL / NCO
      -- -----------------------------------------------------------------
      phase_prev <= phase;
      phase      <= phase + STEP;
      -- The 2-FF input synchronizer above delays rx_s by exactly 2
      -- clk_eth cycles (50 ns) relative to rx_in -- exactly half a 100 ns
      -- bit period, i.e. 128/256 of the phase circle. Every edge-relative
      -- comparison below (correction window, sample point) must be made
      -- against the phase as seen from rx_s's own delayed timeline, so
      -- shift by +128 (mod 256) before comparing to TARGET/SAMPLE_PT
      -- (both of which are defined relative to an undelayed 100 ns bit).
      -- Without this compensation the guaranteed mid-bit transition
      -- (delayed) lands right on the phase-0 boundary -- exactly where
      -- data-dependent boundary edges also land -- so the loop cannot
      -- tell them apart and mis-locks as soon as non-alternating data
      -- (SFD/payload) introduces real boundary edges.
      p_int := to_integer(phase + DELAY_COMP);

      if (rx_s xor rx_prev) = '1' then
        -- NOTE: signal assignment is last-write-wins, not accumulating,
        -- so any correction here must stand in for (not add to) the
        -- normal "+STEP" free-run increment above for this cycle.
        if p_int > (TARGET - WINDOW) and p_int < (TARGET + WINDOW) then
          -- Edge landed near the expected mid-bit point: pull the
          -- accumulator a fraction of the way toward TARGET (bang-bang
          -- proportional correction -- NUDGE sets the loop gain). Using
          -- a partial step rather than a hard snap lets several
          -- preamble edges average out ns-level jitter instead of
          -- chasing every single edge exactly.
          -- write back into the (unshifted) phase register: undo the
          -- +DELAY_COMP shift used only for the comparison above.
          if p_int < TARGET then
            phase <= to_unsigned(p_int, N) + NUDGE - DELAY_COMP;
          elsif p_int > TARGET then
            phase <= to_unsigned(p_int, N) - NUDGE - DELAY_COMP;
          else
            phase <= phase + STEP;
          end if;
        end if;
        -- edges outside the window (near the 0/256 bit boundary) are
        -- data-dependent boundary transitions -- ignored by the loop
        -- (the unconditional "phase <= phase + STEP" above still applies
        -- for those cycles).
      end if;

      -- capture the bit value once per bit period at the mid-second-half
      -- sample point
      if p_int >= SAMPLE_PT and sampled = '0' then
        bit_sample <= rx_s;
        sampled    <= '1';
      end if;

      -- wrap detect (phase counter rolled over -> end of bit period)
      if phase < phase_prev then
        sampled <= '0';
        if carrier_r = '1' then
          bit_o_r     <= bit_sample;
          bit_valid_r <= '1';
        end if;
      end if;

      if rst = '1' then
        rx_m1 <= '0'; rx_s <= '0'; rx_prev <= '0';
        no_edge_cnt <= (others => '0');
        edge_run    <= (others => '0');
        carrier_r   <= '0';
        phase       <= (others => '0');
        phase_prev  <= (others => '0');
        sampled     <= '0';
        bit_sample  <= '0';
        bit_o_r     <= '0';
        bit_valid_r <= '0';
      end if;
    end if;
  end process;

end architecture;
