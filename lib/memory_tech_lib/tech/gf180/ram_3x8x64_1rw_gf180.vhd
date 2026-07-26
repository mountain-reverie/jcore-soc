-- tech/gf180 backend for ram_3x8x64_1rw (the Task 6 2 KB cache TAG RAM --
-- the tag WIDENS from 16 to 24 bit when the cache shrinks to 2 KB):
-- instantiates 3x vendor gf180mcu_fd_ip_sram__sram64x8m8wm1 single-port
-- hard-IP SRAM macros (one per 8-bit subword of the 24-bit x 64 word). No
-- row/col tiling needed -- ADDR_WIDTH=6 (64 deep) maps 1:1 onto the macro's
-- native 64-deep A(5:0), and sram64x8 is the SMALLEST macro the PDK ships
-- (see gf180mcu_fd_ip_sram_comp.vhd's header) -- a tight, non-wasteful fit,
-- unlike a fallback to a larger (128x8/256x8) macro would have been.
--
-- jcore's ram_1rw interface is SYNC 1-cycle registered read, matching the
-- vendor macro's registered Q exactly -- no latency adapter required.
--
-- Structure copied verbatim from ram_2x8x256_1rw_gf180.vhd (see that file's
-- header for the full polarity/control-mapping rationale and the
-- symmetric-write PRECONDITION -- identical here, just 3 lanes instead of 2
-- and address width 6 instead of 8):
--   CEN  <= not en
--   GWEN <= not (wr and we(i))
--   WEN  <= (others => '0')
--   A    <= a
--   D    <= dw(lane i's 8 bits)
--   dr(lane i's 8 bits) <= Q
--
-- SYNTH/METRICS-ONLY: this architecture instantiates a black-box vendor
-- macro (see gf180mcu_fd_ip_sram_comp.vhd) that GHDL cannot elaborate to a
-- functional model on its own -- rtl.sh / functional simulation continue to
-- use tech/sim. The gf180_j4mmu FLASH-variant XIP cosim (xip_sim.sh) binds
-- it to the SIM-ONLY behavioral stub in
-- components/memory/tests/gf180_sram_sim_stub.vhd instead (never analyzed
-- into the LibreLane/synth file list), same mechanism as the 256x8/512x8
-- wrappers.
library ieee;
use ieee.std_logic_1164.all;
use work.gf180_sram_comp_pkg.all;

architecture gf180 of ram_3x8x64_1rw is
  signal cen  : std_logic;
  signal gwen : std_logic_vector(2 downto 0);
  constant WEN_ALL_ON : std_logic_vector(7 downto 0) := (others => '0');
begin
  -- rst and margin are not used by the vendor macro (no reset/margin pins);
  -- kept in the port list for drop-in compatibility with the ram_1rw
  -- interface and the tech/sim behavioral model.

  cen <= not en;

  subword_gen: for i in 0 to 2 generate
    gwen(i) <= not (wr and we(i));

    sram_i: \gf180mcu_fd_ip_sram__sram64x8m8wm1\
      port map (
        CLK  => clk,
        CEN  => cen,
        GWEN => gwen(i),
        WEN  => WEN_ALL_ON,
        A    => a,
        D    => dw((i+1)*8-1 downto i*8),
        Q    => dr((i+1)*8-1 downto i*8));
  end generate;
end architecture;
