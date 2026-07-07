-- spi_page_cache: demand-paging XIP cache core for the iCESugar UP5K (Sub-project B).
--
-- Observes the CPU's instruction and data master buses. Any access whose
-- top address bits fall in the flash window (a(31 downto 20) = PC_WIN_BASE's
-- x"400") is decoded here: VA(19 downto 12) is compared against 4 tag
-- registers (pc_tag_array_t). On a HIT the access is served transparently
-- from the matching 4 KB frame (frame_ram), with 1-cycle read latency
-- (ack/d registered the cycle after the hit is detected -- matching the
-- brief's "implement 1-cycle latency and assert accordingly"). On a MISS
-- page_fault_o is driven combinationally that same cycle (en + kind), the
-- faulting VA and STATUS are latched on the next clock edge, and the window
-- bus is NOT acked that cycle.
--
-- Frames are written only by the embedded spi_flash_fill (Task 1) engine,
-- driven from the FILL_CMD/FILL_STATUS MMIO regs -- the CPU cannot write
-- frame_ram directly.
--
-- Frame RAM arbitration (single 1W1R EBR, for iCE40 fit): the frame RAM has
-- ONE registered read port shared by the instr and data windows, with
-- INSTRUCTION-FETCH PRIORITY. In any cycle where an instr window-hit is
-- present, the read port serves the instr address and instr_win_i.ack is
-- asserted (1-cycle later); a colliding data window-hit is NOT served that
-- cycle (data_win_i.ack withheld) so the CPU retries the data read on a
-- later cycle when no instr hit competes. This keeps the store to a single
-- 1W1R block (iCE40 EBR-inferable) instead of a duplicated 1W2R store that
-- would not fit. The fill engine's fr_we is the sole write port. In the
-- standalone tb the instr and data hits are exercised separately (no forced
-- collision); the simultaneous case is covered at Task 7 integration.
--
-- Instruction half-word select (big-endian SH-2, per
-- components/memory/dev_ddr_spram.vhd:62-64): the 32-bit frame word is
-- muxed to the 16-bit instr bus by a(1): a(1)='0' -> bits 31:16 (upper),
-- a(1)='1' -> bits 15:0 (lower). a(1) is registered alongside the hit so it
-- aligns with the registered frame read.
--
-- MMIO regs are decoded combinationally on reg_i.a(4 downto 2) (no read
-- latency), so tests can poll FILL_STATUS/STATUS/FAULT_VA without extra
-- wait states. Register writes are synchronous (each on rising_edge(clk)).

library ieee;
use ieee.std_logic_1164.all;
use ieee.numeric_std.all;
use work.cpu2j0_pack.all;
use work.spi_page_cache_pack.all;

entity spi_page_cache is
  port (
    clk : in std_logic; rst : in std_logic;
    -- observed CPU master buses (address+en) for window decode + fault
    instr_master_o : in cpu_instruction_o_t;
    data_master_o  : in cpu_data_o_t;
    -- window slave responses (transparent hit reads back to the CPU buses)
    instr_win_i : out cpu_instruction_i_t;  -- ack+d for windowed fetches
    data_win_i  : out cpu_data_i_t;         -- ack+d for windowed reads
    win_instr_sel : out std_logic;          -- '1' when instr access is in window (mux select at cpus level)
    win_data_sel  : out std_logic;
    -- sideband fault to the CPU
    page_fault_o : out cpu_page_fault_i_t;
    -- MMIO register slave (from soc_gen device bus)
    reg_i : in  cpu_data_o_t;
    reg_o : out cpu_data_i_t;
    -- config-flash pins (to ice_spi_io d_* side, wired at cpus level)
    d_cs_n : out std_logic; d_sck : out std_logic; d_mosi : out std_logic; d_miso : in std_logic);
end entity;

architecture rtl of spi_page_cache is

  -- MMIO word offsets (reg_i.a(4 downto 2)) -- see spi_page_cache_pack:
  --  000 TAG0  001 TAG1  010 TAG2  011 TAG3
  --  100 FAULT_VA  101 STATUS  110 FILL_CMD  111 FILL_STATUS
  constant SEL_TAG0       : std_logic_vector(2 downto 0) := "000";
  constant SEL_TAG1       : std_logic_vector(2 downto 0) := "001";
  constant SEL_TAG2       : std_logic_vector(2 downto 0) := "010";
  constant SEL_TAG3       : std_logic_vector(2 downto 0) := "011";
  constant SEL_FAULT_VA   : std_logic_vector(2 downto 0) := "100";
  constant SEL_STATUS     : std_logic_vector(2 downto 0) := "101";
  constant SEL_FILL_CMD   : std_logic_vector(2 downto 0) := "110";
  constant SEL_FILL_STAT  : std_logic_vector(2 downto 0) := "111";

  type frame_ram_t is array(0 to 4095) of std_logic_vector(31 downto 0);
  signal frame_ram : frame_ram_t := (others => (others => '0'));

  signal tags : pc_tag_array_t := (others => PC_TAG_RESET);

  signal fault_va_r      : std_logic_vector(31 downto 0) := (others => '0');
  signal status_pending  : std_logic := '0';
  signal status_lastkind : std_logic := '0'; -- '0'=PF_IFETCH, '1'=PF_DREAD

  -- fill engine command/status
  signal fill_start_r : std_logic := '0';
  signal fill_page_r  : std_logic_vector(7 downto 0) := (others => '0');
  signal fill_frame_r : std_logic_vector(1 downto 0) := (others => '0');
  signal fill_busy    : std_logic;
  signal fill_done    : std_logic;

  signal fr_we   : std_logic;
  signal fr_addr : std_logic_vector(11 downto 0);
  signal fr_data : std_logic_vector(31 downto 0);

  -- window decode / compare (combinational)
  signal win_instr_sel_i : std_logic;
  signal win_data_sel_i  : std_logic;

  signal hit_instr     : std_logic_vector(PC_NFRAMES - 1 downto 0);
  signal hit_data      : std_logic_vector(PC_NFRAMES - 1 downto 0);
  signal hit_instr_any : std_logic;
  signal hit_data_any  : std_logic;
  signal miss_instr    : std_logic;
  signal miss_data     : std_logic;

  signal idx_instr : natural range 0 to PC_NFRAMES - 1;
  signal idx_data  : natural range 0 to PC_NFRAMES - 1;

  -- registered (1-cycle-latency) window responses. rd_word_r is the single
  -- shared read port's registered 32-bit output; instr/data outputs mux from
  -- it. instr_hi_r is the registered a(1) for big-endian half-word select.
  signal instr_ack_r : std_logic := '0';
  signal data_ack_r  : std_logic := '0';
  signal rd_word_r   : std_logic_vector(31 downto 0) := (others => '0');
  signal instr_hi_r  : std_logic := '0';

  -- combinational read-port arbitration (instr-fetch priority)
  signal rd_index : natural range 0 to 4095;

  signal reg_o_d : std_logic_vector(31 downto 0) := (others => '0');

begin

  ----------------------------------------------------------------------------
  -- Window decode + tag compare (combinational)
  ----------------------------------------------------------------------------
  win_instr_sel_i <= instr_master_o.en when instr_master_o.a(31 downto 20) = x"400" else '0';
  win_data_sel_i  <= data_master_o.rd  when data_master_o.a(31 downto 20)  = x"400" else '0';

  win_instr_sel <= win_instr_sel_i;
  win_data_sel  <= win_data_sel_i;

  gen_hit : for i in 0 to PC_NFRAMES - 1 generate
    hit_instr(i) <= tags(i).valid when tags(i).page = instr_master_o.a(19 downto 12) else '0';
    hit_data(i)  <= tags(i).valid when tags(i).page = data_master_o.a(19 downto 12)  else '0';
  end generate;

  hit_instr_any <= '0' when hit_instr = (hit_instr'range => '0') else '1';
  hit_data_any  <= '0' when hit_data  = (hit_data'range => '0')  else '1';

  miss_instr <= win_instr_sel_i and not hit_instr_any;
  miss_data  <= win_data_sel_i and not hit_data_any;

  -- priority-encode the matching frame (only one tag is expected to match
  -- a given page at a time; first match wins if more than one does)
  process (hit_instr) is
  begin
    idx_instr <= 0;
    for i in 0 to PC_NFRAMES - 1 loop
      if hit_instr(i) = '1' then
        idx_instr <= i;
      end if;
    end loop;
  end process;

  process (hit_data) is
  begin
    idx_data <= 0;
    for i in 0 to PC_NFRAMES - 1 loop
      if hit_data(i) = '1' then
        idx_data <= i;
      end if;
    end loop;
  end process;

  -- sideband fault: combinational this cycle, priority instr-side
  page_fault_o.en   <= miss_instr or miss_data;
  page_fault_o.kind <= PF_IFETCH when miss_instr = '1' else PF_DREAD;

  -- single shared read-port address mux, instr-fetch priority
  rd_index <= idx_instr * 1024 + to_integer(unsigned(instr_master_o.a(11 downto 2)))
              when (win_instr_sel_i and hit_instr_any) = '1'
              else idx_data * 1024 + to_integer(unsigned(data_master_o.a(11 downto 2)));

  ----------------------------------------------------------------------------
  -- Registered: frame RAM write (fill engine), frame RAM reads (hit-serve,
  -- 1-cycle latency), tag/status/fault-latch MMIO writes.
  ----------------------------------------------------------------------------
  process (clk) is
  begin
    if rising_edge(clk) then
      if rst = '1' then
        tags            <= (others => PC_TAG_RESET);
        fault_va_r      <= (others => '0');
        status_pending  <= '0';
        status_lastkind <= '0';
        fill_start_r    <= '0';
        fill_page_r     <= (others => '0');
        fill_frame_r    <= (others => '0');
        instr_ack_r     <= '0';
        data_ack_r      <= '0';
        rd_word_r       <= (others => '0');
        instr_hi_r      <= '0';
      else
        -- fill-engine frame write (only writer of frame_ram)
        if fr_we = '1' then
          frame_ram(to_integer(unsigned(fr_addr))) <= fr_data;
        end if;

        -- single shared read port (instr-fetch priority), 1-cycle latency.
        -- instr hit is served whenever present; a colliding data hit is
        -- withheld this cycle (data_ack low) so the CPU retries later.
        rd_word_r   <= frame_ram(rd_index);
        instr_ack_r <= win_instr_sel_i and hit_instr_any;
        instr_hi_r  <= instr_master_o.a(1);
        data_ack_r  <= (win_data_sel_i and hit_data_any) and
                       not (win_instr_sel_i and hit_instr_any);

        -- fault latch (priority instr-side, matches page_fault_o.kind above)
        if miss_instr = '1' then
          fault_va_r      <= instr_master_o.a & '0';
          status_pending  <= '1';
          status_lastkind <= '0'; -- PF_IFETCH
        elsif miss_data = '1' then
          fault_va_r      <= data_master_o.a;
          status_pending  <= '1';
          status_lastkind <= '1'; -- PF_DREAD
        end if;

        -- fill command strobe defaults low unless a FILL_CMD write happens below
        fill_start_r <= '0';

        -- MMIO writes
        if reg_i.en = '1' and reg_i.wr = '1' then
          case reg_i.a(4 downto 2) is
            when SEL_TAG0 =>
              tags(0).valid <= reg_i.d(8);
              tags(0).page  <= reg_i.d(7 downto 0);
            when SEL_TAG1 =>
              tags(1).valid <= reg_i.d(8);
              tags(1).page  <= reg_i.d(7 downto 0);
            when SEL_TAG2 =>
              tags(2).valid <= reg_i.d(8);
              tags(2).page  <= reg_i.d(7 downto 0);
            when SEL_TAG3 =>
              tags(3).valid <= reg_i.d(8);
              tags(3).page  <= reg_i.d(7 downto 0);
            when SEL_STATUS =>
              if reg_i.d(0) = '1' then
                status_pending <= '0';
              end if;
            when SEL_FILL_CMD =>
              fill_frame_r <= reg_i.d(9 downto 8);
              fill_page_r  <= reg_i.d(7 downto 0);
              fill_start_r <= '1';
            when others =>
              null;
          end case;
        end if;
      end if;
    end if;
  end process;

  -- big-endian half-word select: a(1)='0' -> bits 31:16, a(1)='1' -> bits 15:0
  instr_win_i.ack <= instr_ack_r;
  instr_win_i.d   <= rd_word_r(31 downto 16) when instr_hi_r = '0'
                     else rd_word_r(15 downto 0);
  data_win_i.ack  <= data_ack_r;
  data_win_i.d    <= rd_word_r;

  ----------------------------------------------------------------------------
  -- MMIO reads (combinational)
  ----------------------------------------------------------------------------
  process (reg_i, tags, fault_va_r, status_pending, status_lastkind, fill_busy, fill_done) is
  begin
    reg_o_d <= (others => '0');
    case reg_i.a(4 downto 2) is
      when SEL_TAG0 =>
        reg_o_d(8)          <= tags(0).valid;
        reg_o_d(7 downto 0) <= tags(0).page;
      when SEL_TAG1 =>
        reg_o_d(8)          <= tags(1).valid;
        reg_o_d(7 downto 0) <= tags(1).page;
      when SEL_TAG2 =>
        reg_o_d(8)          <= tags(2).valid;
        reg_o_d(7 downto 0) <= tags(2).page;
      when SEL_TAG3 =>
        reg_o_d(8)          <= tags(3).valid;
        reg_o_d(7 downto 0) <= tags(3).page;
      when SEL_FAULT_VA =>
        reg_o_d <= fault_va_r;
      when SEL_STATUS =>
        reg_o_d(0) <= status_pending;
        reg_o_d(1) <= status_lastkind;
      when SEL_FILL_CMD =>
        reg_o_d <= (others => '0');
      when SEL_FILL_STAT =>
        reg_o_d(0) <= fill_busy;
        reg_o_d(1) <= fill_done;
      when others =>
        null;
    end case;
  end process;

  reg_o.d   <= reg_o_d;
  reg_o.ack <= reg_i.en;

  ----------------------------------------------------------------------------
  -- Task-1 fill engine
  ----------------------------------------------------------------------------
  fill : entity work.spi_flash_fill
    port map (
      clk => clk, rst => rst,
      start => fill_start_r, page => fill_page_r, frame => fill_frame_r,
      busy => fill_busy, done => fill_done,
      fr_we => fr_we, fr_addr => fr_addr, fr_data => fr_data,
      d_cs_n => d_cs_n, d_sck => d_sck, d_mosi => d_mosi, d_miso => d_miso);

end architecture;
