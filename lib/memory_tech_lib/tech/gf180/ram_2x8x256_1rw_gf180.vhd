-- tech/gf180 backend for ram_2x8x256_1rw: instantiates 2x vendor
-- gf180mcu_fd_ip_sram__sram256x8m8wm1 single-port hard-IP SRAM macros (one
-- per 8-bit subword of the 16-bit x 256 word). No row/col tiling needed --
-- ADDR_WIDTH=8 (256 deep) maps 1:1 onto the macro's native 256-deep A(7:0).
--
-- jcore's ram_1rw interface is SYNC 1-cycle registered read, matching the
-- vendor macro's registered Q exactly -- no latency adapter required.
--
-- Polarity / control mapping (see gf180mcu_fd_ip_sram_comp.vhd header and
-- the vendor behavioral .v: write_flag = !CEN & !GWEN & !(&WEN), read_flag
-- = !CEN & GWEN):
--   CEN  <= not en                         -- chip enable, active-low
--   PRECONDITION: this per-subword GWEN assumes both `we` bits are driven
--   identically per cycle (a symmetric write). Both real callers do exactly
--   that (icache_ram.vhd / dcache_ram.vhd tie the subword we bits together),
--   so it matches tech/sim. An ASYMMETRIC write (we bits differing) would let
--   the un-written subword's macro do a READ that cycle, diverging from
--   tech/sim's "no read during any write" semantics -- do NOT reuse this
--   wrapper with independent per-subword write-enables without revisiting this.
--
--   GWEN <= not (wr and we(i))             -- per-subword global write
--                                              enable, active-low; when
--                                              GWEN='1' the macro performs
--                                              a read regardless of WEN
--   WEN  <= (others => '0')                -- per-bit write mask tied to
--                                              "all bits enabled"; GWEN
--                                              alone gates whether this
--                                              subword's macro writes at
--                                              all (jcore's we is only
--                                              per-subword granularity, not
--                                              per-bit, so per-bit WEN
--                                              individual control is unused)
--   A    <= a                              -- address, direct passthrough
--   D    <= dw(hi/lo 8 bits)               -- write data, split per subword
--   dr(hi/lo 8 bits) <= Q                  -- read data, split per subword
--
-- SYNTH/METRICS-ONLY: this architecture instantiates a black-box vendor
-- macro (see gf180mcu_fd_ip_sram_comp.vhd) that GHDL cannot elaborate to a
-- functional model. It is NOT wired into the GHDL functional-sim path --
-- rtl.sh / functional simulation continue to use tech/sim. This backend is
-- selected only by the gf180 synth build.mk for yosys-based synthesis
-- metrics (and eventually real ASIC synthesis/P&R).
library ieee;
use ieee.std_logic_1164.all;
use work.gf180_sram_comp_pkg.all;

architecture gf180 of ram_2x8x256_1rw is
  signal cen  : std_logic;
  signal gwen : std_logic_vector(1 downto 0);
  constant WEN_ALL_ON : std_logic_vector(7 downto 0) := (others => '0');
begin
  -- rst and margin are not used by the vendor macro (no reset/margin pins);
  -- kept in the port list for drop-in compatibility with the ram_1rw
  -- interface and the tech/sim behavioral model.

  cen <= not en;

  subword_gen: for i in 0 to 1 generate
    gwen(i) <= not (wr and we(i));

    sram_i: \gf180mcu_fd_ip_sram__sram256x8m8wm1\
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
