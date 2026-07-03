library ieee;
use ieee.std_logic_1164.all;
use ieee.numeric_std.all;

entity eth_tx_phy is
  port (
    clk_eth : in  std_logic;
    rst     : in  std_logic;
    tx_start: in  std_logic;
    tx_len  : in  unsigned(11 downto 0);
    rd_addr : out unsigned(11 downto 0);
    rd_data : in  std_logic_vector(7 downto 0);
    busy    : out std_logic;
    done    : out std_logic;
    mdi_p   : out std_logic;
    mdi_n   : out std_logic);
end entity;

-- Continuous, gapless Manchester serializer with byte prefetch.
--
-- clk_eth runs at 40 MHz (was 20 MHz); the on-wire Manchester rate must stay
-- 10 Mbps (50 ns/half-bit), so a 1-bit `step` divider gates the FSM
-- (half/bit_idx/state advance and the `diff` output) to move only every
-- OTHER clk_eth cycle -- each half-bit now spans exactly 2 clk_eth cycles
-- (still gapless: the divider never introduces a skipped/extra half-bit,
-- just doubles its cycle count). The `diff` register is simply not written
-- on the "off" cycle, so the differential output holds steady across both
-- cycles of a half-bit. The IDLE-state NLP heartbeat counter (nlp_cnt) is
-- NOT gated by `step` -- it is a pure wall-clock timer unrelated to
-- half-bit framing, so it still decrements once per raw clk_eth cycle
-- (NLP_PERIOD doubled to keep the same ~16 ms real-time period at 2x clock).
--
-- A real iCE40 SB_RAM40 read port is REGISTERED: rd_data is valid one
-- clk_eth cycle after rd_addr is presented, not the same cycle. To keep the
-- Manchester bitstream gapless, this design prefetches the NEXT byte while
-- shifting out the current one: rd_addr is held at byte_idx+1 for the whole
-- 32-cycle (16 half-bit x 2 clk_eth cycles) SEND of the current byte, giving
-- ample margin (31 cycles) to absorb the 1-cycle registered-read latency --
-- MORE slack than before the retiming, not less, since reads still land on
-- state-advance ("step") boundaries which are now 2 cycles apart. The next
-- byte lands in cur_byte on the very same step-boundary that bit_idx/
-- byte_idx roll over -- no LOAD/wait state, no extra held half-bit.
architecture rtl of eth_tx_phy is
  type state_t is (IDLE, PREFETCH, SEND, TPIDL);
  signal st        : state_t := IDLE;
  signal byte_idx  : unsigned(11 downto 0) := (others => '0');
  signal bit_idx   : unsigned(2 downto 0)  := (others => '0');
  signal half      : std_logic := '0';            -- 0=first half-bit, 1=second
  signal cur_byte  : std_logic_vector(7 downto 0) := (others => '0');
  signal nxt_byte  : std_logic_vector(7 downto 0) := (others => '0');
  signal diff      : std_logic_vector(1 downto 0) := "00";  -- (p,n)
  signal nlp_cnt   : unsigned(19 downto 0) := (others => '0');
  signal done_r    : std_logic := '0';
  signal pre_step  : std_logic := '0';  -- PREFETCH: 0=addr just issued, 1=data ready
  signal step      : std_logic := '0';  -- 1-bit clk_eth divider: FSM/diff advance only when '1'
  constant NLP_PERIOD : unsigned(19 downto 0) := to_unsigned(640000 mod 2**20, 20);
begin
  -- byte0's address while priming the pipe before the frame starts;
  -- byte_idx+1 (the NEXT byte) continuously for the whole time the current
  -- byte (byte_idx) is being shifted out -- see architecture comment above.
  rd_addr <= byte_idx when st = PREFETCH else byte_idx + 1;
  mdi_p <= diff(1);
  mdi_n <= diff(0);
  busy  <= '0' when st = IDLE else '1';
  done  <= done_r;

  process (clk_eth) is
    variable b : std_logic;
  begin
    if rising_edge(clk_eth) then
      done_r <= '0';
      step   <= not step;                        -- free-running 2-cycle divider
      case st is
        when IDLE =>
          diff <= "00";                         -- 0 differential (idle)
          if tx_start = '1' and tx_len /= 0 then
            byte_idx <= (others => '0'); bit_idx <= (others => '0');
            half <= '0'; pre_step <= '0'; st <= PREFETCH;
            step <= '0';  -- deterministic phase: PREFETCH always starts idle-cycle first
          else
            if nlp_cnt = 0 then                  -- emit a single + link pulse
              diff <= "10"; nlp_cnt <= NLP_PERIOD;
            else
              nlp_cnt <= nlp_cnt - 1;
            end if;
          end if;
        when PREFETCH =>
          -- 2-step (4 clk_eth cycle) pipeline fill for byte 0, done entirely
          -- before the frame's first Manchester transition -- start-of-frame
          -- latency, never a mid-frame gap. Gated by `step` like SEND so the
          -- half-bit cadence starts on a consistent phase.
          if step = '1' then
            if pre_step = '0' then
              pre_step <= '1';
            else
              cur_byte <= rd_data;
              st <= SEND;
            end if;
          end if;
        when SEND =>
          -- rd_addr already points at byte_idx+1 (concurrent assignment
          -- above); continuously latch its registered-read data so
          -- nxt_byte holds the next byte well ahead of the byte boundary.
          nxt_byte <= rd_data;
          if step = '1' then
            b := cur_byte(to_integer(bit_idx));     -- LSB-first
            if half = '0' then
              diff <= (not b) & b;                  -- first half = ~b (p,n)=(~b,b)
              half <= '1';
            else
              diff <= b & (not b);                  -- second half = b -> mid-bit transition
              half <= '0';
              if bit_idx = 7 then
                bit_idx <= (others => '0');
                if byte_idx = tx_len - 1 then
                  st <= TPIDL;
                else
                  byte_idx <= byte_idx + 1;
                  cur_byte <= nxt_byte;              -- roll straight into next byte, no gap
                  st <= SEND;
                end if;
              else
                bit_idx <= bit_idx + 1;
              end if;
            end if;
          end if;
        when TPIDL =>
          if step = '1' then
            diff <= "10";                           -- end-of-frame + pulse then idle
            st <= IDLE; done_r <= '1'; nlp_cnt <= NLP_PERIOD;
          end if;
      end case;
      if rst = '1' then
        st <= IDLE; diff <= "00"; nlp_cnt <= (others => '0'); done_r <= '0';
        pre_step <= '0'; step <= '0';
      end if;
    end if;
  end process;
end architecture;
