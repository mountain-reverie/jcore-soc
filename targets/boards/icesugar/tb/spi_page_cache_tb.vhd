library ieee;
use ieee.std_logic_1164.all;
use ieee.numeric_std.all;
use work.cpu2j0_pack.all;
use work.spi_page_cache_pack.all;
use work.data_bus_pack.all;

-- Task 1 (spi_flash_fill) unit test: t1_fill drives a behavioral SPI flash
-- model (decoding the Fast-Read 0x0B command straight off spi_flash_fill's
-- d_* pins -- no ice_spi_io/SB_IO in the loop, matching the note that
-- ice_spi_io is only instantiated at the cpus level) and asserts that a full
-- 4 KB page fill lands correctly in the frame EBR write port: big-endian
-- byte->word packing and the expected fr_addr for the given frame index.
entity spi_page_cache_tb is end entity;

architecture sim of spi_page_cache_tb is
  constant CLK_PER : time := 20 ns;

  signal clk   : std_logic := '0';
  signal rst   : std_logic := '1';
  signal start : std_logic := '0';
  signal page  : std_logic_vector(7 downto 0) := (others => '0');
  signal frame : std_logic_vector(1 downto 0) := (others => '0');
  signal busy  : std_logic;
  signal done  : std_logic;

  signal fr_we   : std_logic;
  signal fr_addr : std_logic_vector(11 downto 0);
  signal fr_data : std_logic_vector(31 downto 0);

  signal d_cs_n  : std_logic;
  signal d_sck   : std_logic;
  signal d_mosi  : std_logic;
  signal d_miso  : std_logic := '0';

  signal first_word : std_logic_vector(31 downto 0) := (others => '0');
  signal first_addr : std_logic_vector(11 downto 0) := (others => '0');
  signal first_seen : boolean := false;

  signal last_word  : std_logic_vector(31 downto 0) := (others => '0');
  signal last_addr  : std_logic_vector(11 downto 0) := (others => '0');
  signal we_count   : natural := 0;

  signal done_rises : natural := 0;
  signal done_prev  : std_logic := '0';

  signal t1_done : boolean := false;
  signal t3_done : boolean := false;

  constant FLASH_BASE : std_logic_vector(23 downto 0) := x"100000";

  -- Task 3 (spi_page_cache) signals
  signal pc_rst : std_logic := '1';

  signal pc_instr_o : cpu_instruction_o_t := NULL_INST_O;
  signal pc_data_o  : cpu_data_o_t := NULL_DATA_O;
  signal pc_instr_i : cpu_instruction_i_t;
  signal pc_data_i  : cpu_data_i_t;
  signal pc_win_instr_sel : std_logic;
  signal pc_win_data_sel  : std_logic;
  signal pc_fault : cpu_page_fault_i_t;

  signal pc_reg_o : cpu_data_o_t := NULL_DATA_O;
  signal pc_reg_i : cpu_data_i_t;

  signal pc_d_cs_n : std_logic;
  signal pc_d_sck  : std_logic;
  signal pc_d_mosi : std_logic;
  signal pc_d_miso : std_logic := '0';

  -- MMIO word offsets (byte addresses; only a(4 downto 2) is decoded by the DUT)
  constant OFF_TAG0      : std_logic_vector(31 downto 0) := x"00000000";
  constant OFF_FAULT_VA  : std_logic_vector(31 downto 0) := x"00000010";
  constant OFF_STATUS    : std_logic_vector(31 downto 0) := x"00000014";
  constant OFF_FILL_CMD  : std_logic_vector(31 downto 0) := x"00000018";
  constant OFF_FILL_STAT : std_logic_vector(31 downto 0) := x"0000001C";

  procedure mmio_write(
    signal reg_o : out cpu_data_o_t;
    constant addr : in std_logic_vector(31 downto 0);
    constant data : in std_logic_vector(31 downto 0);
    signal clk : in std_logic) is
  begin
    reg_o.en <= '1'; reg_o.wr <= '1'; reg_o.a <= addr; reg_o.d <= data;
    wait until rising_edge(clk);
    reg_o.en <= '0'; reg_o.wr <= '0';
  end procedure;

  -- flash_mem(k) := k mod 256, per the task's ambiguity-resolution note.
  -- Modeled as a function (rather than a 16 MB array) since only the
  -- low-order byte of the address matters.
  function flash_mem(k : natural) return std_logic_vector is
  begin
    return std_logic_vector(to_unsigned(k mod 256, 8));
  end function;

  ------------------------------------------------------------------------
  -- Task 4: cpu_core smoke test (PAGE_FAULT_ARCH threaded through the SoC
  -- CPU wrapper + master-bus snoop taps). Local component declaration
  -- (rather than `use work.cpu_core_pack.all`) so this tb can name the new
  -- generic/port additions explicitly without pulling in the whole
  -- cpu_core_pack namespace (which also declares cpumreg etc., unrelated to
  -- this smoke test). Bound to the real work.cpu_core(arch) entity, and its
  -- embedded u_cpu bound to the existing cpu_sim_pagefault configuration
  -- (components/cpu/core/cpu_config.vhd), via the block configuration
  -- t4_core_smoke_cfg at the bottom of this file.
  ------------------------------------------------------------------------
  component cpu_core is
    generic (PAGE_FAULT_ARCH : boolean := false);
    port (
      clk : in std_logic;
      rst : in std_logic;
      instr_bus_o : out instr_bus_o_t;
      instr_bus_i : in  instr_bus_i_t;
      data_bus_lock : out std_logic;
      data_bus_o : out data_bus_o_t;
      data_bus_i : in  data_bus_i_t;
      debug_o : out cpu_debug_o_t;
      debug_i : in  cpu_debug_i_t;
      event_o : out cpu_event_o_t;
      event_i : in  cpu_event_i_t;
      data_master_en : out std_logic;
      data_master_ack : out std_logic;
      copro_o : out cop_o_t;
      copro_i : in  cop_i_t;
      page_fault_i : in cpu_page_fault_i_t := NULL_PAGE_FAULT_I;
      instr_master_snoop : out cpu_instruction_o_t;
      data_master_snoop  : out cpu_data_o_t);
  end component;

  signal t4_clk  : std_logic := '0';
  signal t4_rst  : std_logic := '1';
  signal t4_done : boolean := false;

  -- Instruction bus always acks with a SH-2 NOP (0x0009) so the CPU keeps
  -- fetching (and hence toggling instr_master_snoop.en) instead of stalling.
  -- Data bus also always acks (benign all-zero read data) so an incidental
  -- data access (e.g. a stray load in the reset/exception path) doesn't wedge
  -- the smoke test; this tb only asserts fetch activity, not data content.
  signal t4_instr_bus_o : instr_bus_o_t;
  signal t4_instr_bus_i : instr_bus_i_t := (others => (d => x"0009", ack => '1'));
  signal t4_data_bus_lock : std_logic;
  signal t4_data_bus_o : data_bus_o_t;
  signal t4_data_bus_i : data_bus_i_t := (others => (d => (others => '0'), ack => '1'));
  signal t4_debug_o : cpu_debug_o_t;
  signal t4_event_o : cpu_event_o_t;
  signal t4_data_master_en  : std_logic;
  signal t4_data_master_ack : std_logic;
  signal t4_copro_o : cop_o_t;
  signal t4_instr_snoop : cpu_instruction_o_t;
  signal t4_data_snoop  : cpu_data_o_t;
  signal t4_fetch_seen  : boolean := false;

begin

  clk <= not clk after CLK_PER / 2 when not (t1_done and t3_done) else '0';

  dut : entity work.spi_flash_fill
    generic map (FLASH_BASE => FLASH_BASE)
    port map (
      clk => clk, rst => rst,
      start => start, page => page, frame => frame,
      busy => busy, done => done,
      fr_we => fr_we, fr_addr => fr_addr, fr_data => fr_data,
      d_cs_n => d_cs_n, d_sck => d_sck, d_mosi => d_mosi, d_miso => d_miso);

  -- Behavioral SPI flash model (mode 0): while cs_n is low, sample mosi on
  -- sck rising edges. Decode 8 (cmd) + 24 (addr) bits, then 8 dummy bits,
  -- then drive flash_mem(addr+i) MSB-first on miso, i incrementing each byte.
  flash_model : process
    variable shreg   : std_logic_vector(31 downto 0);
    variable bitcnt  : natural;
    variable addr    : natural;
    variable tx_byte : std_logic_vector(7 downto 0);
    variable byte_i  : natural;
  begin
    d_miso <= '0';
    -- wait for cs_n to go low
    wait until d_cs_n = '0';

    -- shift in cmd+addr (32 bits) on sck rising edges
    shreg := (others => '0');
    for i in 0 to 31 loop
      wait until rising_edge(d_sck);
      shreg := shreg(30 downto 0) & d_mosi;
    end loop;
    addr := to_integer(unsigned(shreg(23 downto 0)));

    -- 8 dummy clocks
    for i in 0 to 7 loop
      wait until rising_edge(d_sck);
    end loop;

    -- stream bytes until cs_n deasserted
    byte_i := 0;
    loop
      tx_byte := flash_mem(addr + byte_i);
      for b in 7 downto 0 loop
        if d_cs_n = '1' then
          exit;
        end if;
        d_miso <= tx_byte(b);
        wait until rising_edge(d_sck) or d_cs_n = '1';
        if d_cs_n = '1' then
          exit;
        end if;
      end loop;
      byte_i := byte_i + 1;
      if d_cs_n = '1' then
        exit;
      end if;
    end loop;
    wait;
  end process;

  -- capture the first frame write
  capture : process (clk)
  begin
    if rising_edge(clk) then
      if fr_we = '1' then
        if not first_seen then
          first_word <= fr_data;
          first_addr <= fr_addr;
          first_seen <= true;
        end if;
        last_word <= fr_data;
        last_addr <= fr_addr;
        we_count  <= we_count + 1;
      end if;
      -- count rising edges of `done`
      if done = '1' and done_prev = '0' then
        done_rises <= done_rises + 1;
      end if;
      done_prev <= done;
    end if;
  end process;

  t1_fill : process
  begin
    rst <= '1';
    wait for CLK_PER * 4;
    rst <= '0';
    wait for CLK_PER * 4;

    page  <= x"02";
    frame <= "01";
    start <= '1';
    wait for CLK_PER;
    start <= '0';

    wait until done = '1' for 2 ms;
    assert done = '1' report "spi_flash_fill did not complete (done never asserted)" severity failure;
    -- let the clk-edge capture process register the final fr_we/done edge
    wait until rising_edge(clk);
    wait until rising_edge(clk);

    assert first_word = x"00010203" report "fill word0 packing" severity failure;
    assert first_addr = std_logic_vector(to_unsigned(1024, 12)) report "fill addr" severity failure;

    -- last word (index 1023): flash bytes at FLASH_BASE + (page<<12) + 1023*4
    -- = 0x102FFC..0x102FFF -> (k mod 256) = FC,FD,FE,FF, packed big-endian.
    assert last_word = x"FCFDFEFF" report "fill last-word packing" severity failure;
    -- frame "01" -> 1*1024 + 1023 = 2047
    assert last_addr = std_logic_vector(to_unsigned(2047, 12)) report "fill last addr" severity failure;

    -- exactly 1024 word writes (one per 32-bit word of the 4 KB page)
    assert we_count = 1024 report "fill fr_we pulse count" severity failure;

    -- done pulsed exactly once, and busy returned low
    assert done_rises = 1 report "done rose exactly once, got " & integer'image(done_rises) severity failure;
    assert busy = '0' report "busy returned to 0 after fill" severity failure;

    report "Test Passed t1_fill" severity note;
    t1_done <= true;
    wait;
  end process;

  ------------------------------------------------------------------------
  -- Task 3: spi_page_cache DUT (instantiates its own spi_flash_fill inside)
  ------------------------------------------------------------------------
  pc_dut : entity work.spi_page_cache
    port map (
      clk => clk, rst => pc_rst,
      instr_master_o => pc_instr_o,
      data_master_o  => pc_data_o,
      instr_win_i => pc_instr_i,
      data_win_i  => pc_data_i,
      win_instr_sel => pc_win_instr_sel,
      win_data_sel  => pc_win_data_sel,
      page_fault_o => pc_fault,
      reg_i => pc_reg_o,
      reg_o => pc_reg_i,
      d_cs_n => pc_d_cs_n, d_sck => pc_d_sck, d_mosi => pc_d_mosi, d_miso => pc_d_miso);

  -- Same behavioral SPI flash model as t1, but wired to the page cache's
  -- own d_* pins (its embedded spi_flash_fill engine drives these).
  pc_flash_model : process
    variable shreg   : std_logic_vector(31 downto 0);
    variable addr    : natural;
    variable tx_byte : std_logic_vector(7 downto 0);
    variable byte_i  : natural;
  begin
    pc_d_miso <= '0';
    loop
      wait until pc_d_cs_n = '0';

      shreg := (others => '0');
      for i in 0 to 31 loop
        wait until rising_edge(pc_d_sck);
        shreg := shreg(30 downto 0) & pc_d_mosi;
      end loop;
      addr := to_integer(unsigned(shreg(23 downto 0)));

      for i in 0 to 7 loop
        wait until rising_edge(pc_d_sck);
      end loop;

      byte_i := 0;
      outer : loop
        tx_byte := flash_mem(addr + byte_i);
        for b in 7 downto 0 loop
          if pc_d_cs_n = '1' then
            exit outer;
          end if;
          pc_d_miso <= tx_byte(b);
          wait until rising_edge(pc_d_sck) or pc_d_cs_n = '1';
          if pc_d_cs_n = '1' then
            exit outer;
          end if;
        end loop;
        byte_i := byte_i + 1;
      end loop;
    end loop;
  end process;

  -- All three t3_* checks run sequentially in a single process (rather than
  -- as separate concurrent processes) so that the shared DUT input signals
  -- (pc_instr_o/pc_data_o/pc_reg_o) have exactly one driver at a time -- the
  -- CPU/register buses are not resolved multi-driver signals in real HW,
  -- so a single sequential driver process is the correct model here.
  t3_tests : process
    variable va32 : std_logic_vector(31 downto 0);
  begin
    wait until t1_done;

    pc_rst <= '1';
    wait for CLK_PER * 4;
    pc_rst <= '0';
    wait for CLK_PER * 4;

    ------------------------------------------------------------------
    -- t3_miss_fault: no tags valid -> instruction fetch into the flash
    -- window must miss.
    ------------------------------------------------------------------
    va32 := x"40002000";
    pc_instr_o.en <= '1';
    pc_instr_o.a  <= va32(31 downto 1);
    wait until rising_edge(clk);
    wait for 1 ns; -- let combinational outputs settle after the edge

    assert pc_win_instr_sel = '1' report "t3_miss_fault: win_instr_sel" severity failure;
    assert pc_fault.en = '1' report "t3_miss_fault: page_fault_o.en" severity failure;
    assert pc_fault.kind = PF_IFETCH report "t3_miss_fault: page_fault_o.kind" severity failure;

    wait until rising_edge(clk); -- let the fault latch (FAULT_VA/STATUS) register
    wait for 1 ns;

    pc_instr_o.en <= '0';
    wait until rising_edge(clk);

    -- MMIO read FAULT_VA
    pc_reg_o.en <= '1'; pc_reg_o.wr <= '0'; pc_reg_o.a <= OFF_FAULT_VA;
    wait for 1 ns;
    assert pc_reg_i.d = x"40002000" report "t3_miss_fault: FAULT_VA readback" severity failure;

    -- MMIO read STATUS
    pc_reg_o.a <= OFF_STATUS;
    wait for 1 ns;
    assert pc_reg_i.d(0) = '1' report "t3_miss_fault: STATUS.pending" severity failure;
    pc_reg_o.en <= '0';

    report "Test Passed t3_miss_fault" severity note;

    ------------------------------------------------------------------
    -- t3_fill_then_hit: fill frame 0 with page 0x02, tag it, and confirm
    -- a fetch to the corresponding VA now hits instead of faulting.
    ------------------------------------------------------------------
    wait for CLK_PER * 4;

    -- FILL_CMD: frame=0, page=0x02
    mmio_write(pc_reg_o, OFF_FILL_CMD, x"00000002", clk);

    -- poll FILL_STATUS until done
    loop
      pc_reg_o.en <= '1'; pc_reg_o.wr <= '0'; pc_reg_o.a <= OFF_FILL_STAT;
      wait for 1 ns;
      exit when pc_reg_i.d(1) = '1';
      wait until rising_edge(clk);
    end loop;
    pc_reg_o.en <= '0';

    -- TAG0 = {valid=1, page=0x02}
    mmio_write(pc_reg_o, OFF_TAG0, x"00000102", clk);

    -- fetch 0x40002004 -> should hit, no fault
    va32 := x"40002004";
    pc_instr_o.en <= '1';
    pc_instr_o.a  <= va32(31 downto 1);
    wait until rising_edge(clk); -- hit detected combinationally this cycle
    wait for 1 ns;
    assert pc_fault.en = '0' report "t3_fill_then_hit: unexpected fault" severity failure;

    wait until rising_edge(clk); -- 1-cycle read latency: ack/d register here
    wait for 1 ns;
    assert pc_instr_i.ack = '1' report "t3_fill_then_hit: instr_win_i.ack" severity failure;
    -- flash byte k = k mod 256; word at FLASH_BASE+0x2004 = bytes 04,05,06,07
    -- packed big-endian into frame_ram = 0x04050607. instr_win_i.d is 16 bits
    -- (cpu_instruction_i_t): a(1)='0' selects the UPPER half (bits 31:16).
    assert pc_instr_i.d = x"0405" report "t3_fill_then_hit: instr_win_i.d even(a1=0)" severity failure;

    pc_instr_o.en <= '0';
    wait until rising_edge(clk);

    -- ODD half-word: fetch 0x40002006, a(1)='1' selects the LOWER half (bits 15:0)
    va32 := x"40002006";
    pc_instr_o.en <= '1';
    pc_instr_o.a  <= va32(31 downto 1);
    wait until rising_edge(clk); -- hit detected combinationally this cycle
    wait for 1 ns;
    assert pc_fault.en = '0' report "t3_fill_then_hit: unexpected fault (odd)" severity failure;

    wait until rising_edge(clk); -- 1-cycle read latency
    wait for 1 ns;
    assert pc_instr_i.ack = '1' report "t3_fill_then_hit: instr_win_i.ack (odd)" severity failure;
    assert pc_instr_i.d = x"0607" report "t3_fill_then_hit: instr_win_i.d odd(a1=1)" severity failure;

    pc_instr_o.en <= '0';
    wait until rising_edge(clk);

    report "Test Passed t3_fill_then_hit" severity note;

    ------------------------------------------------------------------
    -- t3_data_dread: same as the miss test but on the data bus with
    -- rd='1', targeting an untagged page -> PF_DREAD.
    ------------------------------------------------------------------
    wait for CLK_PER * 4;

    pc_data_o.en <= '1';
    pc_data_o.rd <= '1';
    pc_data_o.a  <= x"40003000";
    wait until rising_edge(clk);
    wait for 1 ns;

    assert pc_win_data_sel = '1' report "t3_data_dread: win_data_sel" severity failure;
    assert pc_fault.en = '1' report "t3_data_dread: page_fault_o.en" severity failure;
    assert pc_fault.kind = PF_DREAD report "t3_data_dread: page_fault_o.kind" severity failure;

    pc_data_o.en <= '0';
    pc_data_o.rd <= '0';
    wait until rising_edge(clk);

    report "Test Passed t3_data_dread" severity note;
    t3_done <= true;
    wait;
  end process;

  ------------------------------------------------------------------------
  -- Task 4: t4_core_smoke -- elaborates cpu_core with PAGE_FAULT_ARCH => true
  -- and page_fault_i tied to NULL_PAGE_FAULT_I, and asserts fetch activity
  -- (instr_master_snoop.en pulses at least once) is observable on the new
  -- snoop tap. Runs on its own free-running clock/reset (t4_clk/t4_rst),
  -- independent of the t1/t3 shared `clk`, so it is unaffected by those
  -- tests' timing/stop conditions.
  ------------------------------------------------------------------------
  t4_dut : cpu_core
    generic map (PAGE_FAULT_ARCH => true)
    port map (
      clk => t4_clk, rst => t4_rst,
      instr_bus_o => t4_instr_bus_o, instr_bus_i => t4_instr_bus_i,
      data_bus_lock => t4_data_bus_lock,
      data_bus_o => t4_data_bus_o, data_bus_i => t4_data_bus_i,
      debug_o => t4_debug_o, debug_i => CPU_DEBUG_NOP,
      event_o => t4_event_o, event_i => NULL_CPU_EVENT_I,
      data_master_en => t4_data_master_en, data_master_ack => t4_data_master_ack,
      copro_o => t4_copro_o, copro_i => NULL_COPR_I,
      page_fault_i => NULL_PAGE_FAULT_I,
      instr_master_snoop => t4_instr_snoop,
      data_master_snoop => t4_data_snoop);

  t4_clkgen : process
  begin
    while not t4_done loop
      t4_clk <= not t4_clk;
      wait for CLK_PER / 2;
    end loop;
    wait;
  end process;

  t4_core_smoke : process
  begin
    t4_rst <= '1';
    wait for CLK_PER * 4;
    t4_rst <= '0';

    for i in 0 to 300 loop
      wait until rising_edge(t4_clk);
      if t4_instr_snoop.en = '1' then
        t4_fetch_seen <= true;
      end if;
    end loop;

    assert t4_fetch_seen
      report "t4_core_smoke: instr_master_snoop.en never asserted (no fetch activity observed)"
      severity failure;

    report "Test Passed t4_core_smoke" severity note;
    t4_done <= true;
    wait;
  end process;

end architecture;

-- Block configuration binding t4_dut's local `cpu_core` component to the real
-- work.cpu_core(arch) entity, and (descending further) its embedded u_cpu to
-- the same variant selection as the existing cpu_sim_pagefault configuration
-- (components/cpu/core/cpu_config.vhd: decode_core PAGE_FAULT_ARCH=>true,
-- MMU_ARCH=>false; two_bank regfile; comb shifter) -- inlined here (rather
-- than `use configuration work.cpu_sim_pagefault` directly) because this tb's
-- analyze order (needed to satisfy cpu_config.vhd's own unconditional
-- references to the DSP-ALU prototype variant, core/mult_ice40dsp.vhd +
-- core/dsp_arith.vhd) leaves entity `mult` with two analyzed architectures
-- (stru, ice40dsp); cpu_sim_pagefault's default binding for u_mult (it has no
-- explicit "for u_mult" entry) would then resolve to whichever was analyzed
-- last, not necessarily `stru`. Binding u_mult explicitly here removes that
-- ambiguity regardless of analyze order.
-- Elaborate/run this configuration name (not the bare entity) so the binding
-- takes effect: `ghdl -r --std=93 t4_core_smoke_cfg ...`.
-- Top-level configuration "of cpu" -- same shape/scope as cpu_config.vhd's
-- own cpu_sim_pagefault, so the "mult"/"decode"/"datapath"/"register_file"/
-- "shifter" component names resolve via the standard inherited-scope rule
-- (visibility of cpu's architecture `stru`), same as every other config in
-- that file. Referenced below by name (`use configuration work.
-- t4_cpu_pagefault_cfg`), matching the established repo pattern (e.g.
-- icesugar/cpus_config.vhd's `for u_cpu : cpu use configuration work.
-- cpu_synth_j1_dsp;`) rather than inlining cpu's internals directly inside
-- t4_core_smoke_cfg (which GHDL rejects: the component names are not visible
-- at that nesting point without this file having its own `use
-- cpu2j0_components_pack.all` etc.).
configuration t4_cpu_pagefault_cfg of cpu is
  for stru
    for u_mult : mult use entity work.mult(stru); end for;
    for u_decode : decode use configuration work.cpu_decode_direct_pagefault; end for;
    for u_datapath : datapath use entity work.datapath(stru);
      for stru
        for u_regfile : register_file use entity work.register_file(two_bank); end for;
        for u_shifter : shifter use entity work.shifter(comb); end for;
      end for;
    end for;
  end for;
end configuration;

configuration t4_core_smoke_cfg of spi_page_cache_tb is
  for sim
    for t4_dut : cpu_core
      use entity work.cpu_core(arch);
      for arch
        for u_cpu : cpu
          use configuration work.t4_cpu_pagefault_cfg;
        end for;
      end for;
    end for;
  end for;
end configuration;
