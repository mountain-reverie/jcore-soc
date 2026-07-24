-- tech/gf180 backend for bootram_infer (the boot ROM/RAM behind the CPU's
-- instruction/data bus, see components/memory/bootram_infer.vhd): tiles
-- vendor gf180mcu_fd_ip_sram__sram512x8m8wm1 single-port hard-IP SRAM macros
-- (4 byte-lanes x ceil(WORDS/512) depth-tiles) to cover the c_addr_width-14
-- (WORDS=4096, 32-bit) boot RAM used by gf180_j4mmu.
--
-- WHY 32 macros (not 64, i.e. no per-port duplication): bootram_infer serves
-- TWO logical ports (db_i/db_o data port: read+write, and ibus_i/ibus_o
-- instruction port: read-only) from ONE shared word array. A single-port
-- vendor macro can only service ONE (address, op) per cycle. This backend
-- keeps the area at 32 macros (matching ram_2x8x2048_2rw_gf180's tiling
-- convention rather than duplicating the whole array per port) by
-- time-multiplexing: the DATA port has priority every cycle it is enabled
-- (db_i.en='1'); the INSTRUCTION port is only physically served on cycles
-- the data port is idle (db_i.en='0').
--
-- ENABLING PRECONDITION (mirrors ram_2x8x2048_2rw_gf180's documented
-- precondition for the cache RAM): during the boot phase bootram_infer
-- serves, jcore's CPU issues at most one live bus request per cycle to the
-- boot RAM (either a data access OR an instruction fetch, never both
-- asserted with `en`='1' in the same cycle) -- this is the same "single
-- physical access per cycle" assumption the cache gf180 wrapper relies on.
-- If a caller ever asserts db_i.en and ibus_i.en simultaneously, this
-- backend serves the DATA port and still asserts ibus_o.ack, so the
-- instruction port receives the data port's just-served word (the wrong
-- tile's data reinterpreted as a fetch) -- NOT merely a stale/old value --
-- unlike (inferred), which independently registers both ports every cycle
-- from the same array. This is a SYNTH/METRICS-ONLY
-- concern: the committed functional-sim/FPGA path keeps (inferred) as the
-- default architecture (see bootram_gf180_config.vhd -- an ADDITIONAL
-- config binding, not a change to the default), so no functional flow is
-- affected. See task-1-report.md for how the equivalence tb exercises this.
--
-- REGISTERED READ-DATA / EDGE ALIGNMENT: (inferred) drives its register
-- process off `falling_edge(clk)` (so ack, asserted combinationally the
-- same cycle en rises, is seen valid together with data at the NEXT rising
-- edge -- see bootram_infer.vhd's own timing-contract comment). The vendor
-- macro's Q is a standard rising-edge-triggered synchronous read (1-cycle
-- latency). To align latency exactly (no extra pipeline stage needed), this
-- wrapper clocks every macro from `not clk` -- the macro's rising edge then
-- coincides with the system clock's falling edge, giving byte-identical
-- timing to (inferred) with zero extra cycles.
--
-- TILING: WORDS = 2**(c_addr_width-2) (4096 for c_addr_width=14). Word index
-- di = a(c_addr_width-1 downto 2). Depth-tiles = WORDS/512 (8 for
-- c_addr_width=14); tile select = di(WORD_BITS-1 downto 9) (top bits of the
-- word index); macro-native address = di(8 downto 0). 4 byte-lanes (di's
-- 32-bit word split into bytes 0..3, matching db_i.we(0..3)/d bit ranges).
-- Total macros = 4 lanes x TILES depth-tiles = 32 for c_addr_width=14.
--
-- Polarity / control mapping (see gf180mcu_fd_ip_sram_comp.vhd header):
--   CLK  <= not clk                      -- edge-alignment, see above
--   CEN  <= not (port_active and row_match)
--   GWEN <= not (op_write and we(lane))  -- write-enable per byte lane
--   WEN  <= "00000000"                   -- per-bit mask unused (byte-only we)
--   A    <= active native address (9 bits), muxed by which port is being served
--   D    <= db_i.d (byte lane slice)     -- write data always from the data port
--   Q    -> assembled into db_o.d / ibus_o.d via the registered tile-select
--           mux (see ram_2x8x2048_2rw_gf180.vhd's identical row_sel_reg
--           technique for why the mux index must be the REGISTERED, not
--           combinational, tile select)
--
-- READ-DURING-WRITE: unlike (inferred) (which is READ_FIRST -- d_word
-- captures the pre-write value even on a write cycle, via non-blocking
-- signal-assignment ordering), this backend does not guarantee any
-- particular Q value on a write cycle (GWEN=0 puts the macro in write mode;
-- real vendor macros typically do not also return valid old-data on Q that
-- same cycle). This is a known, accepted divergence: jcore's bus protocol
-- never asserts `rd` and `wr` in the same cycle at the boot RAM (see
-- bootram_infer_tb.vhd's write-then-separately-read pattern, mirrored by
-- the equivalence tb here), so it is never observed by any real caller.
library ieee;
use ieee.std_logic_1164.all;
use ieee.numeric_std.all;
use work.cpu2j0_pack.all;
use work.gf180_sram_comp_pkg.all;

architecture gf180 of bootram_infer is
  constant WORD_BITS : integer := c_addr_width - 2; -- width of word index di
  constant WORDS     : integer := 2 ** WORD_BITS;
  constant TILES      : integer := WORDS / 512; -- >=1 for c_addr_width in 11..14

  signal clk_n : std_logic;

  -- Data (db) port decode
  signal db_word_idx  : unsigned(WORD_BITS - 1 downto 0);
  signal db_tile_sel  : integer range 0 to TILES - 1;
  signal db_native_a  : std_logic_vector(8 downto 0);

  -- Instruction (ibus) port decode
  signal ibus_word_idx : unsigned(WORD_BITS - 1 downto 0);
  signal ibus_tile_sel : integer range 0 to TILES - 1;
  signal ibus_native_a : std_logic_vector(8 downto 0);

  -- Arbitration: data port has priority whenever enabled (see header);
  -- instruction port is only physically served when the data port is idle.
  signal serving_db    : std_logic;
  signal serving_ibus  : std_logic;
  signal active_tile   : integer range 0 to TILES - 1;
  signal active_a      : std_logic_vector(8 downto 0);
  signal op_write      : std_logic;

  signal serving_db_reg   : std_logic := '0';
  signal serving_ibus_reg : std_logic := '0';
  signal active_tile_reg  : integer range 0 to TILES - 1 := 0;
  signal i_half_reg       : std_logic := '0';

  constant WEN_ALL_ON : std_logic_vector(7 downto 0) := (others => '0');

  type lane_data_t is array (0 to 3) of std_logic_vector(7 downto 0);
  type all_q_t is array (0 to TILES - 1) of lane_data_t;
  signal q_arr : all_q_t;

  signal q_word : std_logic_vector(31 downto 0);
begin
  clk_n <= not clk;

  -- Decode both ports every cycle (combinational), regardless of enable --
  -- mirrors (inferred), which also computes `di` unconditionally.
  db_word_idx   <= unsigned(db_i.a(c_addr_width - 1 downto 2));
  db_tile_sel   <= to_integer(db_word_idx(WORD_BITS - 1 downto 9));
  db_native_a   <= std_logic_vector(db_word_idx(8 downto 0));

  ibus_word_idx <= unsigned(ibus_i.a(c_addr_width - 1 downto 2));
  ibus_tile_sel <= to_integer(ibus_word_idx(WORD_BITS - 1 downto 9));
  ibus_native_a <= std_logic_vector(ibus_word_idx(8 downto 0));

  -- Arbitration (see ENABLING PRECONDITION in the header): data port wins.
  serving_db   <= db_i.en;
  serving_ibus <= ibus_i.en and not db_i.en;

  active_tile <= db_tile_sel  when serving_db = '1' else ibus_tile_sel;
  active_a    <= db_native_a  when serving_db = '1' else ibus_native_a;
  op_write    <= db_i.en and db_i.wr;

  reg_sel : process(clk_n)
  begin
    if rising_edge(clk_n) then
      serving_db_reg   <= serving_db;
      serving_ibus_reg <= serving_ibus;
      active_tile_reg  <= active_tile;
      i_half_reg       <= ibus_i.a(1);
    end if;
  end process;

  tile_gen : for t in 0 to TILES - 1 generate
    signal row_match : std_logic;
    signal cen       : std_logic;
  begin
    row_match <= '1' when active_tile = t else '0';
    cen <= not ((db_i.en or ibus_i.en) and row_match);

    lane_gen : for l in 0 to 3 generate
      signal gwen : std_logic;
      -- Explicit binding (component specs at the outer architecture level
      -- don't reach into generate-statement scopes): for GHDL sim (the
      -- equivalence tb) this binds to whichever entity named
      -- \gf180mcu_fd_ip_sram__sram512x8m8wm1\ is visible in `work` at
      -- analysis time (see components/memory/tests/gf180_sram_sim_stub.vhd,
      -- the sim-only behavioral stub); the real synth/P&R flow does not go
      -- through GHDL elaboration for this macro (see file header) so this
      -- spec has no effect there.
      for sram_i : \gf180mcu_fd_ip_sram__sram512x8m8wm1\
        use entity work.\gf180mcu_fd_ip_sram__sram512x8m8wm1\;
    begin
      gwen <= not (op_write and db_i.we(l));

      sram_i : \gf180mcu_fd_ip_sram__sram512x8m8wm1\
        port map (
          CLK  => clk_n,
          CEN  => cen,
          GWEN => gwen,
          WEN  => WEN_ALL_ON,
          A    => active_a,
          D    => db_i.d((l + 1) * 8 - 1 downto l * 8),
          Q    => q_arr(t)(l));
    end generate;
  end generate;

  q_word <= q_arr(active_tile_reg)(3) & q_arr(active_tile_reg)(2) &
            q_arr(active_tile_reg)(1) & q_arr(active_tile_reg)(0);

  -- Data port: q_word is only meaningful on cycles the data port was the one
  -- physically served (serving_db_reg='1'); ack is combinational = en,
  -- exactly matching (inferred).
  db_o.d   <= q_word;
  db_o.ack <= db_i.en;

  -- Instruction port: big-endian halfword select, same convention as
  -- (inferred) (a(1)='0' -> high half). q_word is only meaningful on cycles
  -- the instruction port was the one physically served
  -- (serving_ibus_reg='1').
  ibus_o.d   <= q_word(31 downto 16) when i_half_reg = '0' else q_word(15 downto 0);
  ibus_o.ack <= ibus_i.en;
end architecture;
