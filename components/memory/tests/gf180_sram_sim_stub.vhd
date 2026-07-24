-- SIM-ONLY behavioral stub of the GF180MCU vendor hard-IP SRAM macro
-- `gf180mcu_fd_ip_sram__sram512x8m8wm1`, used ONLY by
-- bootram_equiv_tb.vhd. No in-tree behavioral VHDL/Verilog model of this
-- vendor cell exists in this repo (the tech/gf180 wrappers for the cache RAM
-- -- ram_2x8x256_1rw_gf180.vhd / ram_2x8x2048_2rw_gf180.vhd -- are
-- documented as "SYNTH/METRICS-ONLY... GHDL cannot elaborate to a functional
-- model" and are NOT wired into any GHDL sim path; the real vendor behavioral
-- model + .lib/.lef/.gds are supplied out-of-tree from the PDK at
-- synthesis/P&R time, see gf180_sram_comp_pkg.vhd's header). This stub
-- exists purely so the equivalence testbench can exercise the tiling/
-- decode/lane-assembly logic in bootram_2Nx8_gf180.vhd against a
-- functional model; it MUST NOT be used for anything other than this tb (it
-- is not part of any build.mk / build_asic.mk file list).
--
-- Modeled semantics (matches the port-list comments in
-- gf180_sram_comp_pkg.vhd and gf180mcu_fd_ip_sram_comp.vhd): CEN/GWEN/WEN
-- active-low; synchronous, rising-edge-triggered; Q is REGISTERED (1-cycle
-- read latency) -- write-first is NOT modeled (Q is simply held/undefined
-- during write cycles' own edge, updated to the read value on subsequent
-- read cycles), which matches the "no guarantee on read-during-write" note
-- in bootram_2Nx8_gf180.vhd.
library ieee;
use ieee.std_logic_1164.all;
use ieee.numeric_std.all;
use work.gf180_sram_comp_pkg.all;

entity \gf180mcu_fd_ip_sram__sram512x8m8wm1\ is
  port (
    CLK  : in  std_logic;
    CEN  : in  std_logic;
    GWEN : in  std_logic;
    WEN  : in  std_logic_vector(7 downto 0);
    A    : in  std_logic_vector(8 downto 0);
    D    : in  std_logic_vector(7 downto 0);
    Q    : out std_logic_vector(7 downto 0));
end entity;

architecture sim of \gf180mcu_fd_ip_sram__sram512x8m8wm1\ is
  type mem_t is array (0 to 511) of std_logic_vector(7 downto 0);
  signal mem : mem_t := (others => (others => '0'));
  signal q_r : std_logic_vector(7 downto 0) := (others => '0');
begin
  process(CLK)
    variable idx : integer;
  begin
    if rising_edge(CLK) then
      if CEN = '0' then
        idx := to_integer(unsigned(A));
        if GWEN = '0' then
          -- write: WEN is tied all-'0' (all bits enabled) by every caller in
          -- this repo, so a bit-accurate per-bit WEN mask isn't modeled.
          mem(idx) <= D;
        else
          q_r <= mem(idx);
        end if;
      end if;
    end if;
  end process;

  Q <= q_r;
end architecture;
