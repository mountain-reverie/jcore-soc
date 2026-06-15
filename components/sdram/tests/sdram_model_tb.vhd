library ieee;
use ieee.std_logic_1164.all;
use ieee.numeric_std.all;
use work.sdram_pkg.all;

-- Validates sdram_model itself: a legal init+ACTIVE+WRITE+READ sequence must
-- round-trip data at the CAS latency. (An illegal sequence - e.g. READ before
-- ACTIVE - makes the model assert; verified manually during bring-up.)
entity sdram_model_tb is end entity;

architecture sim of sdram_model_tb is
  constant CL : integer := 2;
  signal clk : std_logic := '0';
  signal cke : std_logic := '1';
  signal cs_n, ras_n, cas_n, we_n : std_logic := '1';
  signal ba  : std_logic_vector(1 downto 0) := "00";
  signal a   : std_logic_vector(12 downto 0) := (others => '0');
  signal dqm : std_logic_vector(1 downto 0) := "00";
  signal dq  : std_logic_vector(15 downto 0);
  signal drive : std_logic_vector(15 downto 0) := (others => 'Z');
  signal done : boolean := false;
begin
  uut : entity work.sdram_model(behave)
    generic map (CAS_LATENCY => CL, MEM_WORDS => 256)
    port map (clk=>clk, cke=>cke, cs_n=>cs_n, ras_n=>ras_n, cas_n=>cas_n, we_n=>we_n,
              ba=>ba, a=>a, dqm=>dqm, dq=>dq);
  dq  <= drive;
  clk <= not clk after 10 ns when not done else '0';

  stim : process
    procedure issue(c : std_logic_vector(3 downto 0); bk : integer; ad : integer) is
    begin
      cs_n <= c(3); ras_n <= c(2); cas_n <= c(1); we_n <= c(0);
      ba <= std_logic_vector(to_unsigned(bk, 2));
      a  <= std_logic_vector(to_unsigned(ad, 13));
      wait until rising_edge(clk);
    end procedure;
  begin
    -- legal init
    issue(CMD_PRE, 0, 1024);     -- a(10)=1 -> PRECHARGE ALL
    issue(CMD_REF, 0, 0);
    issue(CMD_REF, 0, 0);
    issue(CMD_LMR, 0, 0);
    issue(CMD_NOP, 0, 0);
    -- ACTIVE bank0 row5
    issue(CMD_ACT, 0, 5);
    -- WRITE col3 = 0xBEEF
    drive <= x"BEEF";
    issue(CMD_WRITE, 0, 3);
    drive <= (others => 'Z');
    issue(CMD_NOP, 0, 0);
    -- READ col3, data appears CL cycles after the READ edge
    issue(CMD_READ, 0, 3);
    for i in 1 to CL loop
      issue(CMD_NOP, 0, 0);
    end loop;
    wait for 1 ns;               -- let dq settle past the read-pipeline delta
    assert dq = x"BEEF" report "model round-trip failed" severity failure;
    report "sdram_model_tb PASSED" severity note;
    done <= true;
    wait;
  end process;
end architecture;
