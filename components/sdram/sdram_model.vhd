library ieee;
use ieee.std_logic_1164.all;
use ieee.numeric_std.all;
use work.sdram_pkg.all;

-- Sim-only behavioral SDR SDRAM. Decodes the SDRAM command set, tracks the
-- open row/bank, models CAS-latency read data, stores byte-masked writes, and
-- asserts on protocol violations (access before init, READ/WRITE without an
-- ACTIVE to that bank). The backing array is bounded by MEM_WORDS (the tb only
-- touches a small window per bank); addresses fold in via mod MEM_WORDS so a
-- high row still round-trips while the controller's full-width ba/a decode is
-- checked independently by the testbench (bus snoop).
entity sdram_model is
  generic (
    CAS_LATENCY : integer := 2;
    ROW_BITS    : integer := SDR_ROW_BITS;
    COL_BITS    : integer := SDR_COL_BITS;
    BANK_BITS   : integer := SDR_BANK_BITS;
    MEM_WORDS   : integer := 4096);
  port (
    clk   : in    std_logic;
    cke   : in    std_logic;
    cs_n  : in    std_logic;
    ras_n : in    std_logic;
    cas_n : in    std_logic;
    we_n  : in    std_logic;
    ba    : in    std_logic_vector(BANK_BITS - 1 downto 0);
    a     : in    std_logic_vector(ROW_BITS - 1 downto 0);
    dqm   : in    std_logic_vector(1 downto 0);
    dq    : inout std_logic_vector(15 downto 0));
end entity;

architecture behave of sdram_model is
  type mem_t is array (0 to (2**BANK_BITS) * MEM_WORDS - 1) of std_logic_vector(15 downto 0);
  shared variable mem : mem_t := (others => (others => '0'));

  signal open_bank : integer := -1;
  signal open_row  : integer := -1;
  signal inited    : boolean := false;
  signal pre_all_seen : boolean := false;
  signal ref_count : integer := 0;

  type rdpipe_t is array (0 to CAS_LATENCY) of std_logic_vector(15 downto 0);
  signal rdpipe  : rdpipe_t := (others => (others => '0'));
  signal rdvalid : std_logic_vector(0 to CAS_LATENCY) := (others => '0');

  function idx(bk, row, col : integer) return integer is
  begin
    return bk * MEM_WORDS + ((row * (2**COL_BITS) + col) mod MEM_WORDS);
  end function;
begin
  dq <= rdpipe(CAS_LATENCY) when rdvalid(CAS_LATENCY) = '1' else (others => 'Z');

  process(clk)
    variable cmd : std_logic_vector(3 downto 0);
    variable bk, col : integer;
    variable wword : std_logic_vector(15 downto 0);
  begin
    if rising_edge(clk) then
      for i in CAS_LATENCY downto 1 loop
        rdpipe(i)  <= rdpipe(i-1);
        rdvalid(i) <= rdvalid(i-1);
      end loop;
      rdvalid(0) <= '0';
      if cke = '1' then
        cmd := cs_n & ras_n & cas_n & we_n;
        case cmd is
          when CMD_PRE =>
            if a(10) = '1' then pre_all_seen <= true; end if;
            open_bank <= -1; open_row <= -1;
          when CMD_REF =>
            ref_count <= ref_count + 1;
          when CMD_LMR =>
            assert pre_all_seen report "LMR before PRECHARGE-ALL" severity failure;
            inited <= true;
          when CMD_ACT =>
            assert inited report "ACTIVE before init complete" severity failure;
            open_bank <= to_integer(unsigned(ba));
            open_row  <= to_integer(unsigned(a));
          when CMD_READ =>
            assert inited report "READ before init" severity failure;
            assert open_bank = to_integer(unsigned(ba))
              report "READ without ACTIVE to that bank" severity failure;
            bk  := to_integer(unsigned(ba));
            col := to_integer(unsigned(a(COL_BITS - 1 downto 0)));
            rdpipe(0)  <= mem(idx(bk, open_row, col));
            rdvalid(0) <= '1';
          when CMD_WRITE =>
            assert inited report "WRITE before init" severity failure;
            assert open_bank = to_integer(unsigned(ba))
              report "WRITE without ACTIVE to that bank" severity failure;
            bk  := to_integer(unsigned(ba));
            col := to_integer(unsigned(a(COL_BITS - 1 downto 0)));
            wword := mem(idx(bk, open_row, col));
            if dqm(0) = '0' then wword(7 downto 0)  := dq(7 downto 0);  end if;
            if dqm(1) = '0' then wword(15 downto 8) := dq(15 downto 8); end if;
            mem(idx(bk, open_row, col)) := wword;
          when others => null;
        end case;
      end if;
    end if;
  end process;
end architecture;
