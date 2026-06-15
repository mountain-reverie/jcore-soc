library ieee;
use ieee.std_logic_1164.all;
use ieee.numeric_std.all;
use work.cpu2j0_pack.all;
use work.sdram_pkg.all;

-- Self-checking controller testbench: ctrl + iocells + model on one clock.
-- Scenarios are added per task. A command-bus snoop records the init sequence
-- and counts AUTO-REFRESH (for the refresh scenario).
entity sdram_ctrl_tb is end entity;

architecture sim of sdram_ctrl_tb is
  constant CL : integer := 2;
  signal clk : std_logic := '0';
  signal rst : std_logic := '1';
  signal req : cpu_data_o_t := (en=>'0', a=>(others=>'0'), rd=>'0', wr=>'0', we=>"0000", d=>(others=>'0'));
  signal bst : std_logic := '0';
  signal resp : cpu_data_i_t;
  signal ack_r : std_logic;
  signal c : sdram_cmd_t;
  signal dq_o, dq_i : std_logic_vector(15 downto 0);
  signal dq_oe : std_logic;
  signal dq : std_logic_vector(15 downto 0);
  signal done : boolean := false;

  -- snoop flags
  signal saw_pre_all, saw_lmr : boolean := false;
  signal ref_count : integer := 0;
  signal pre_all_before_lmr, refs_before_lmr_ok : boolean := false;
begin
  uut : entity work.sdram_ctrl(rtl)
    generic map (CAS_LATENCY => CL, T_INIT => 20, T_RC => 8, T_RCD => 2,
                 T_RP => 2, T_RFC => 8, T_MRD => 2, T_REFI => 1024)
    port map (clk=>clk, rst=>rst, req=>req, bst=>bst, resp=>resp, ack_r=>ack_r,
              cmd=>c, dq_o=>dq_o, dq_oe=>dq_oe, dq_i=>dq_i);

  io : entity work.sdram_iocells(rtl)
    port map (dq_o=>dq_o, dq_oe=>dq_oe, dq_i=>dq_i, dq=>dq);

  mem : entity work.sdram_model(behave)
    generic map (CAS_LATENCY => CL, MEM_WORDS => 4096)
    port map (clk=>clk, cke=>c.cke, cs_n=>c.cs_n, ras_n=>c.ras_n, cas_n=>c.cas_n,
              we_n=>c.we_n, ba=>c.ba, a=>c.a, dqm=>c.dqm, dq=>dq);

  clk <= not clk after 10 ns when not done else '0';

  -- command-bus snoop
  snoop : process(clk)
    variable cmd4 : std_logic_vector(3 downto 0);
  begin
    if rising_edge(clk) and c.cke = '1' then
      cmd4 := c.cs_n & c.ras_n & c.cas_n & c.we_n;
      if cmd4 = CMD_PRE and c.a(10) = '1' then saw_pre_all <= true; end if;
      if cmd4 = CMD_REF then ref_count <= ref_count + 1; end if;
      if cmd4 = CMD_LMR then
        saw_lmr <= true;
        if saw_pre_all then pre_all_before_lmr <= true; end if;
        if ref_count >= 2 then refs_before_lmr_ok <= true; end if;
      end if;
    end if;
  end process;

  stim : process
    -- hold the request asserted until the controller acks (held-bus requester,
    -- robust to controller-not-ready), then drop it.
    procedure do_write(addr, data : std_logic_vector(31 downto 0);
                       we : std_logic_vector(3 downto 0)) is
    begin
      req.en <= '1'; req.a <= addr; req.wr <= '1'; req.rd <= '0';
      req.we <= we; req.d <= data; bst <= '0';
      wait until resp.ack = '1' for 10 us;
      assert resp.ack = '1' report "write ack timeout" severity failure;
      req.en <= '0'; req.wr <= '0'; req.we <= "0000"; req.d <= (others => '0');
      wait until rising_edge(clk);
    end procedure;

    procedure do_read(addr : std_logic_vector(31 downto 0);
                      data : out std_logic_vector(31 downto 0)) is
    begin
      req.en <= '1'; req.a <= addr; req.rd <= '1'; req.wr <= '0'; bst <= '0';
      wait until resp.ack = '1' for 10 us;
      assert resp.ack = '1' report "read ack timeout" severity failure;
      data := resp.d;
      req.en <= '0'; req.rd <= '0';
      wait until rising_edge(clk);
    end procedure;

    variable rdata : std_logic_vector(31 downto 0);
  begin
    wait until rising_edge(clk);
    rst <= '0';
    -- Scenario 1: init sequence runs (PRECHARGE-ALL, 2x REF, LMR, in order),
    -- and the model accepts it (no model assert fires during init).
    wait until saw_lmr for 50 us;
    assert saw_lmr report "init never completed (no LMR seen)" severity failure;
    assert pre_all_before_lmr report "LMR without a preceding PRECHARGE-ALL" severity failure;
    assert refs_before_lmr_ok report "LMR without >=2 AUTO-REFRESH" severity failure;
    report "ctrl init OK" severity note;

    -- Scenario 2: single 32-bit write then read back.
    do_write(x"00000020", x"CAFEBABE", "1111");
    do_read (x"00000020", rdata);
    assert rdata = x"CAFEBABE" report "single rw failed" severity failure;
    report "single rw OK" severity note;

    done <= true;
    wait;
  end process;
end architecture;
