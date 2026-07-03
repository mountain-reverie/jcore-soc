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

architecture rtl of eth_tx_phy is
  type state_t is (IDLE, LOAD, SEND, TPIDL);
  signal st        : state_t := IDLE;
  signal byte_idx  : unsigned(11 downto 0) := (others => '0');
  signal bit_idx   : unsigned(2 downto 0)  := (others => '0');
  signal half      : std_logic := '0';            -- 0=first half-bit, 1=second
  signal cur_byte  : std_logic_vector(7 downto 0) := (others => '0');
  signal diff      : std_logic_vector(1 downto 0) := "00";  -- (p,n)
  signal nlp_cnt   : unsigned(18 downto 0) := (others => '0');
  signal done_r    : std_logic := '0';
  constant NLP_PERIOD : unsigned(18 downto 0) := to_unsigned(320000 mod 2**19, 19);
begin
  rd_addr <= byte_idx;
  mdi_p <= diff(1);
  mdi_n <= diff(0);
  busy  <= '0' when st = IDLE else '1';
  done  <= done_r;

  process (clk_eth) is
    variable b : std_logic;
  begin
    if rising_edge(clk_eth) then
      done_r <= '0';
      case st is
        when IDLE =>
          diff <= "00";                         -- 0 differential (idle)
          if tx_start = '1' and tx_len /= 0 then
            byte_idx <= (others => '0'); bit_idx <= (others => '0');
            half <= '0'; st <= LOAD;
          else
            if nlp_cnt = 0 then                  -- emit a single + link pulse
              diff <= "10"; nlp_cnt <= NLP_PERIOD;
            else
              nlp_cnt <= nlp_cnt - 1;
            end if;
          end if;
        when LOAD =>
          cur_byte <= rd_data;                    -- rd_data valid (addr set last cycle)
          st <= SEND;
        when SEND =>
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
                byte_idx <= byte_idx + 1; st <= LOAD;
              end if;
            else
              bit_idx <= bit_idx + 1;
            end if;
          end if;
        when TPIDL =>
          diff <= "10";                           -- end-of-frame + pulse then idle
          st <= IDLE; done_r <= '1'; nlp_cnt <= NLP_PERIOD;
      end case;
      if rst = '1' then st <= IDLE; diff <= "00"; nlp_cnt <= (others => '0'); done_r <= '0'; end if;
    end if;
  end process;
end architecture;
