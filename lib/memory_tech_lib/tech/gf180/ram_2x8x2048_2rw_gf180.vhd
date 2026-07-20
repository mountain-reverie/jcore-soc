-- tech/gf180 backend for ram_2x8x2048_2rw (the cache DATA RAM): tiles 8x
-- vendor gf180mcu_fd_ip_sram__sram512x8m8wm1 single-port hard-IP SRAM macros
-- (4 row-banks x 2 byte-columns) to cover the 16-bit x 2048-deep interface,
-- and MUXES the 1R+1W ram_2rw interface onto those single-port macros.
--
-- ENABLING PRECONDITION (spike-verified, 0 collisions across 1019 dcache
-- scoreboard tests): in single-clock mode (cache_clkmode_sc, clk0 = clk1 --
-- see tech/inferred/ram_2rw_infer.vhd's identical assumption) the cache's
-- port0-READ and port1-WRITE NEVER occur in the same cycle; the blocking
-- cache FSM guarantees this. So a single physical access per cycle -- a MUX,
-- not a true dual-port -- correctly serves both ports:
--   physical op = WRITE  when (en1 and wr1) = '1'
--               = READ   when en0 = '1' (and not writing)
--               = idle   otherwise
-- Port1 in cache usage is WRITE-ONLY (dr1 is never read by icache_ram.vhd /
-- dcache_ram.vhd) -- this wrapper drives dr1 <= '0' and does NOT support
-- port1 reads. Do NOT reuse this wrapper for a caller that reads port1 or
-- that can issue port0-read/port1-write in the same cycle -- both break the
-- single-physical-access-per-cycle assumption above.
--
-- TILING: 16 bits = 2 byte-columns (col0 = bits 7:0, col1 = bits 15:8,
-- matching ram_2x8x256_1rw_gf180's subword_gen lane convention: i=0 -> low
-- byte, i=1 -> high byte). 2048 deep = 4 row-banks of 512 (addr(10:9)
-- selects the row-bank, addr(8:0) is the macro-native address). So this is
-- 4 rows x 2 cols = 8 sram512x8 macros total. Only the addressed row-bank's
-- CEN is asserted each cycle (the other 3 idle, CEN='1').
--
-- Polarity / control mapping per macro (see gf180mcu_fd_ip_sram_comp.vhd
-- header and the vendor behavioral .v -- same semantics as the 256x8
-- variant, just A(8:0)):
--   CLK  <= clk0                          -- single-clock assumption; clk1
--                                             is unused (like tech/inferred)
--   CEN  <= not (op_active and row_match) -- chip enable, active-low; only
--                                             the addressed row-bank enables
--   GWEN <= not (op_write and we1(col))   -- per-column global write enable,
--                                             active-low; write data always
--                                             comes from port1 (dw1/we1),
--                                             matching the WRITE=port1 /
--                                             READ=port0 cache convention
--   WEN  <= "00000000"                    -- per-bit mask tied to "all bits
--                                             enabled"; GWEN alone gates
--                                             whether a column writes (jcore's
--                                             we is only per-byte granularity)
--   A    <= col_addr = (a1 or a0)(8 downto 0), muxed by op_write
--   D    <= dw1(hi/lo 8 bits)             -- write data always from port1
--   Q    -> dr0(hi/lo 8 bits), muxed by the REGISTERED row select (see below)
--
-- REGISTERED READ-DATA MUX: the vendor macro's Q is registered (1-cycle sync
-- read, matching ram_2rw's dr0 semantics exactly -- see the 256x8 wrapper's
-- header for the same latency note). Because only ONE row-bank is enabled
-- per cycle, this wrapper must remember WHICH row-bank was addressed on the
-- cycle whose Q is now valid, and mux dr0 from that row's Q pair -- using
-- the CURRENT cycle's row_sel (combinational) would pick the wrong bank's
-- (stale) Q. row_sel_reg is row_sel delayed by exactly one clk0 edge, so
-- dr0 always reads back the correct bank regardless of which bank is being
-- addressed THIS cycle.
--
-- SYNTH/METRICS-ONLY: like the 256x8 wrapper, this architecture instantiates
-- a black-box vendor macro that GHDL cannot elaborate to a functional model.
-- It is NOT wired into the GHDL functional-sim path (tech/sim / tech/inferred
-- stay the sim/FPGA backends) -- selected only by the gf180 synth build for
-- yosys-based synthesis metrics.
library ieee;
use ieee.std_logic_1164.all;
use ieee.numeric_std.all;
use work.gf180_sram_comp_pkg.all;

architecture gf180 of ram_2x8x2048_2rw is
  constant WEN_ALL_ON : std_logic_vector(7 downto 0) := (others => '0');

  signal op_write : std_logic;
  signal op_active : std_logic;
  signal row_sel     : std_logic_vector(1 downto 0);
  signal row_sel_reg : std_logic_vector(1 downto 0);
  signal col_addr    : std_logic_vector(8 downto 0);

  type row_data_t is array (0 to 1) of std_logic_vector(7 downto 0);
  type all_q_t is array (0 to 3) of row_data_t;
  signal q_arr : all_q_t;
begin
  -- rst0/rst1/margin0/margin1 are not used by the vendor macro (no reset/
  -- margin pins); kept in the port list for drop-in compatibility with the
  -- ram_2rw interface and the tech/sim/tech/inferred behavioral models.
  -- clk1 is unused: single-clock precondition (clk0 = clk1), see header.

  op_write  <= en1 and wr1;
  op_active <= op_write or en0;
  row_sel   <= a1(10 downto 9) when op_write = '1' else a0(10 downto 9);
  col_addr  <= a1(8 downto 0)  when op_write = '1' else a0(8 downto 0);

  reg_row_sel: process(clk0)
  begin
    if clk0'event and clk0 = '1' then
      row_sel_reg <= row_sel;
    end if;
  end process;

  row_gen: for r in 0 to 3 generate
    signal row_match : std_logic;
    signal cen        : std_logic;
  begin
    row_match <= '1' when row_sel = std_logic_vector(to_unsigned(r, 2)) else '0';
    cen <= not (op_active and row_match);

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
          Q    => q_arr(r)(c));
    end generate;
  end generate;

  dr0 <= q_arr(to_integer(unsigned(row_sel_reg)))(1) &
         q_arr(to_integer(unsigned(row_sel_reg)))(0);

  -- Port1 is write-only in cache usage (see PRECONDITION above); this
  -- backend does not support reading port1.
  dr1 <= (others => '0');
end architecture;
