library ieee;
use ieee.std_logic_1164.all;
use ieee.numeric_std.all;
use work.cpu2j0_pack.all;
use work.sdram_pkg.all;

-- ASIC-portable SDR-SDRAM controller. Upward: the ddr_ram_mux contract
-- (req:cpu_data_o_t, bst, resp:cpu_data_i_t, ack_r). Downward: single-edge
-- SDRAM pins (sdram_cmd_t + dq_o/dq_oe/dq_i; the inout dq lives in
-- sdram_iocells). A 32-bit word = 2 x 16-bit beats. Closed-row page policy
-- (PRECHARGE after each transaction).
entity sdram_ctrl is
  generic (
    CAS_LATENCY : integer := 2;
    T_INIT : integer := 20;    -- power-up wait (sim-short; ~100us in M1b)
    T_RC   : integer := 8;
    T_RCD  : integer := 2;
    T_RP   : integer := 2;
    T_RFC  : integer := 8;
    T_MRD  : integer := 2;
    T_REFI : integer := 1024);
  port (
    clk  : in  std_logic;
    rst  : in  std_logic;
    req   : in  cpu_data_o_t;
    bst   : in  std_logic;
    resp  : out cpu_data_i_t;
    ack_r : out std_logic;
    cmd   : out sdram_cmd_t;
    dq_o  : out std_logic_vector(15 downto 0);
    dq_oe : out std_logic;
    dq_i  : in  std_logic_vector(15 downto 0));
end entity;

architecture rtl of sdram_ctrl is
  type state_t is (S_WAIT, S_PRE_ALL, S_REF1, S_REF2, S_LMR, S_IDLE,
                   S_RCDW, S_WR0, S_WR1, S_WACK,
                   S_RD0, S_RD0W, S_RD1, S_RD1W, S_RACK, S_PRE);
  signal state, after_st : state_t := S_WAIT;
  signal tmr : integer range 0 to 65535 := T_INIT;
  -- read-data capture latency: data is stable CAS_LATENCY+2 edges after a READ
  -- state is entered (1 cycle to register the command + CAS_LATENCY model
  -- pipeline + 1 settle). Capture when the dwait countdown reaches 1.
  constant RD_WAIT : integer := CAS_LATENCY + 2;
  signal dwait : integer range 0 to 31 := 0;

  -- latched request
  signal lbank : std_logic_vector(SDR_BANK_BITS - 1 downto 0) := (others => '0');
  signal lrow  : std_logic_vector(SDR_ROW_BITS - 1 downto 0) := (others => '0');
  signal lcol  : std_logic_vector(SDR_COL_BITS - 1 downto 0) := (others => '0');
  signal lwe   : std_logic_vector(3 downto 0) := (others => '0');
  signal ld    : std_logic_vector(31 downto 0) := (others => '0');
  signal rd_lo : std_logic_vector(15 downto 0) := (others => '0');

  signal r_cmd : sdram_cmd_t := (cke=>'1', cs_n=>'1', ras_n=>'1', cas_n=>'1',
                                 we_n=>'1', ba=>(others=>'0'), a=>(others=>'0'), dqm=>"11");

  function colpad(col : std_logic_vector) return std_logic_vector is
    variable r : std_logic_vector(SDR_ROW_BITS - 1 downto 0) := (others => '0');
  begin
    r(SDR_COL_BITS - 1 downto 0) := col;  -- a(10)=0 => no auto-precharge
    return r;
  end function;
begin
  cmd <= r_cmd;

  process(clk)
    procedure setcmd(c : std_logic_vector(3 downto 0)) is
    begin
      r_cmd.cs_n <= c(3); r_cmd.ras_n <= c(2); r_cmd.cas_n <= c(1); r_cmd.we_n <= c(0);
    end procedure;
    variable sa : sdram_addr_t;
  begin
    if rising_edge(clk) then
      r_cmd.cke <= '1'; r_cmd.dqm <= "11";
      setcmd(CMD_NOP);
      resp.d <= (others => '0'); resp.ack <= '0'; ack_r <= '0';
      dq_o <= (others => '0'); dq_oe <= '0';

      if rst = '1' then
        tmr <= T_INIT; after_st <= S_PRE_ALL; state <= S_WAIT;
      else
        case state is
          when S_WAIT =>
            if tmr <= 1 then state <= after_st; else tmr <= tmr - 1; end if;
          when S_PRE_ALL =>
            setcmd(CMD_PRE); r_cmd.a <= (10 => '1', others => '0');
            tmr <= T_RP; after_st <= S_REF1; state <= S_WAIT;
          when S_REF1 =>
            setcmd(CMD_REF); tmr <= T_RFC; after_st <= S_REF2; state <= S_WAIT;
          when S_REF2 =>
            setcmd(CMD_REF); tmr <= T_RFC; after_st <= S_LMR; state <= S_WAIT;
          when S_LMR =>
            setcmd(CMD_LMR);
            r_cmd.a <= std_logic_vector(to_unsigned(CAS_LATENCY * 16, SDR_ROW_BITS));
            tmr <= T_MRD; after_st <= S_IDLE; state <= S_WAIT;

          when S_IDLE =>
            if req.en = '1' then
              sa := sdram_addr(req.a);
              lbank <= sa.bank; lrow <= sa.row; lcol <= sa.col;
              lwe <= req.we; ld <= req.d;
              setcmd(CMD_ACT); r_cmd.ba <= sa.bank; r_cmd.a <= sa.row;
              tmr <= T_RCD;
              if req.wr = '1' then after_st <= S_WR0; else after_st <= S_RD0; end if;
              state <= S_RCDW;
            end if;
          when S_RCDW =>
            if tmr <= 1 then state <= after_st; else tmr <= tmr - 1; end if;

          -- single write: 2 halfword beats
          when S_WR0 =>
            setcmd(CMD_WRITE); r_cmd.ba <= lbank; r_cmd.a <= colpad(lcol);
            dq_o <= ld(15 downto 0); dq_oe <= '1';
            r_cmd.dqm <= (not lwe(1)) & (not lwe(0));
            state <= S_WR1;
          when S_WR1 =>
            setcmd(CMD_WRITE); r_cmd.ba <= lbank;
            r_cmd.a <= colpad(std_logic_vector(unsigned(lcol) + 1));
            dq_o <= ld(31 downto 16); dq_oe <= '1';
            r_cmd.dqm <= (not lwe(3)) & (not lwe(2));
            state <= S_WACK;
          when S_WACK =>
            resp.ack <= '1';                      -- single-write ack (no ack_r)
            state <= S_PRE;

          -- single read: 2 halfword beats, capture at CAS latency
          when S_RD0 =>
            setcmd(CMD_READ); r_cmd.ba <= lbank; r_cmd.a <= colpad(lcol);
            r_cmd.dqm <= "00";
            dwait <= RD_WAIT; state <= S_RD0W;
          when S_RD0W =>
            if dwait <= 1 then rd_lo <= dq_i; state <= S_RD1;
            else dwait <= dwait - 1; end if;
          when S_RD1 =>
            setcmd(CMD_READ); r_cmd.ba <= lbank;
            r_cmd.a <= colpad(std_logic_vector(unsigned(lcol) + 1));
            r_cmd.dqm <= "00";
            dwait <= RD_WAIT; state <= S_RD1W;
          when S_RD1W =>
            if dwait <= 1 then
              resp.d <= dq_i & rd_lo;             -- high & low
              resp.ack <= '1'; ack_r <= '1';      -- single-read ack + ack_r
              state <= S_PRE;
            else dwait <= dwait - 1; end if;

          when S_PRE =>
            setcmd(CMD_PRE); r_cmd.ba <= lbank; r_cmd.a <= (others => '0');
            tmr <= T_RP; after_st <= S_IDLE; state <= S_WAIT;

          when others =>
            state <= S_IDLE;
        end case;
      end if;
    end if;
  end process;
end architecture;
