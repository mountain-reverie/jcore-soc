-- tech/gf180 backend for ram_2x8x512_2rw (the Task 6 2 KB cache DATA RAM):
-- instantiates 2x vendor gf180mcu_fd_ip_sram__sram512x8m8wm1 single-port
-- hard-IP SRAM macros (one per byte-column of the 16-bit x 512 word) and
-- MUXES the 1R+1W ram_2rw interface onto those single-port macros. No
-- row tiling needed -- ADDR_WIDTH=9 (512 deep) maps 1:1 onto the macro's
-- native 512-deep A(8:0) (unlike the 2048-deep sibling
-- ram_2x8x2048_2rw_gf180.vhd, which needs 4 row-banks of this same macro).
--
-- ENABLING PRECONDITION (same as ram_2x8x2048_2rw_gf180.vhd -- see that
-- file's header for the full spike-verified rationale): in single-clock
-- mode (clk0 = clk1) the cache's port0-READ and port1-WRITE never occur in
-- the same cycle, so a single physical access per cycle (a MUX, not a true
-- dual-port) correctly serves both ports:
--   physical op = WRITE  when (en1 and wr1) = '1'
--               = READ   when en0 = '1' (and not writing)
--               = idle   otherwise
-- Port1 in cache usage is WRITE-ONLY (dr1 is never read by icache_ram.vhd /
-- dcache_ram.vhd) -- this wrapper drives dr1 <= '0' and does NOT support
-- port1 reads. Do NOT reuse this wrapper for a caller that reads port1 or
-- that can issue port0-read/port1-write in the same cycle.
--
-- TILING: 16 bits = 2 byte-columns (col0 = bits 7:0, col1 = bits 15:8,
-- matching the sibling wrappers' lane convention). 512 deep = exactly 1
-- row-bank of 512, so (unlike the 2048-deep sibling) no row_sel/row_match
-- logic is needed -- both macros are always enabled together whenever an
-- access is active.
--
-- Polarity / control mapping per macro (see gf180mcu_fd_ip_sram_comp.vhd
-- header and ram_2x8x2048_2rw_gf180.vhd -- same semantics, just no row
-- selection):
--   CLK  <= clk0                          -- single-clock assumption; clk1
--                                             is unused (like the sibling)
--   CEN  <= not op_active                 -- chip enable, active-low
--   GWEN <= not (op_write and we1(col))   -- per-column global write enable,
--                                             active-low; write data always
--                                             comes from port1 (dw1/we1)
--   WEN  <= "00000000"                    -- per-bit mask tied to "all bits
--                                             enabled"
--   A    <= col_addr = (a1 or a0)(8 downto 0), muxed by op_write
--   D    <= dw1(hi/lo 8 bits)             -- write data always from port1
--   Q    -> dr0(hi/lo 8 bits), REGISTERED (1-cycle sync read, matching
--                                             ram_2rw's dr0 semantics
--                                             exactly -- no extra bank-select
--                                             register needed since there is
--                                             only one bank)
--
-- SYNTH/METRICS-ONLY: like the sibling wrappers, this architecture
-- instantiates a black-box vendor macro that GHDL cannot elaborate to a
-- functional model on its own; the gf180_j4mmu FLASH-variant XIP cosim
-- binds it to the SIM-ONLY behavioral stub in
-- components/memory/tests/gf180_sram_sim_stub.vhd instead (never analyzed
-- into the LibreLane/synth file list).
library ieee;
use ieee.std_logic_1164.all;
use work.gf180_sram_comp_pkg.all;

architecture gf180 of ram_2x8x512_2rw is
  constant WEN_ALL_ON : std_logic_vector(7 downto 0) := (others => '0');

  signal op_write  : std_logic;
  signal op_active : std_logic;
  signal cen       : std_logic;
  signal col_addr  : std_logic_vector(8 downto 0);

  type row_data_t is array (0 to 1) of std_logic_vector(7 downto 0);
  signal q_arr : row_data_t;
begin
  -- rst0/rst1/margin0/margin1 are not used by the vendor macro (no reset/
  -- margin pins); kept in the port list for drop-in compatibility with the
  -- ram_2rw interface and the tech/sim/tech/inferred behavioral models.
  -- clk1 is unused: single-clock precondition (clk0 = clk1), see header.

  op_write  <= en1 and wr1;
  op_active <= op_write or en0;
  col_addr  <= a1(8 downto 0) when op_write = '1' else a0(8 downto 0);
  cen       <= not op_active;

  col_gen: for c in 0 to 1 generate
    signal gwen : std_logic;
  begin
    gwen <= not (op_write and we1(c));

    sram_i: \gf180mcu_fd_ip_sram__sram512x8m8wm1\
      port map (
        CLK  => clk0,
        CEN  => cen,
        GWEN => gwen,
        WEN  => WEN_ALL_ON,
        A    => col_addr,
        D    => dw1((c+1)*8-1 downto c*8),
        Q    => q_arr(c));
  end generate;

  dr0 <= q_arr(1) & q_arr(0);

  -- Port1 is write-only in cache usage (see PRECONDITION above); this
  -- backend does not support reading port1.
  dr1 <= (others => '0');
end architecture;
