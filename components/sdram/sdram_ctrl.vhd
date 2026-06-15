library ieee;
use ieee.std_logic_1164.all;
use ieee.numeric_std.all;
use work.cpu2j0_pack.all;
use work.sdram_pkg.all;

-- ASIC-portable SDR-SDRAM controller. Upward: the ddr_ram_mux contract
-- (req:cpu_data_o_t, bst, resp:cpu_data_i_t, ack_r). Downward: single-edge
-- SDRAM pins (sdram_cmd_t + dq_o/dq_oe/dq_i, the inout dq lives in
-- sdram_iocells). Built incrementally: INIT (this), single R/W, burst, refresh.
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
  type state_t is (S_WAIT, S_PRE_ALL, S_REF1, S_REF2, S_LMR, S_IDLE);
  signal state, after_st : state_t := S_WAIT;
  signal tmr : integer range 0 to 65535 := T_INIT;
  signal r_cmd : sdram_cmd_t := (cke=>'1', cs_n=>'1', ras_n=>'1', cas_n=>'1',
                                 we_n=>'1', ba=>(others=>'0'), a=>(others=>'0'), dqm=>"11");
begin
  cmd <= r_cmd;

  process(clk)
    procedure setcmd(c : std_logic_vector(3 downto 0)) is
    begin
      r_cmd.cs_n <= c(3); r_cmd.ras_n <= c(2); r_cmd.cas_n <= c(1); r_cmd.we_n <= c(0);
    end procedure;
  begin
    if rising_edge(clk) then
      -- registered defaults each cycle
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
            null;  -- request handling added in Task 5
          when others =>
            state <= S_IDLE;
        end case;
      end if;
    end if;
  end process;
end architecture;
