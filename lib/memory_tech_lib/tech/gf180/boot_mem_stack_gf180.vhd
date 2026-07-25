-- tech/gf180 backend for bootram_infer(boot_mem) (see components/memory/
-- boot_mem.vhd): the SILICON-CORRECT boot memory used by the FLASH/XIP
-- variant of gf180_j4mmu (bound via bootram_infer(boot_mem_gf180),
-- c_addr_width=>12, see targets/asic/gf180_j4mmu/cpus_one_m0_gf180_arch.vhd).
--
-- SCRATCHPAD REMOVAL (boot-memory refinement): this architecture used to
-- split the window into a read-only constant-ROM lane (0x000-0x7FF) plus
-- a 2 KB writable stack lane (0x800-0xFFF) backed by 4x vendor
-- gf180mcu_fd_ip_sram__sram512x8m8wm1 hard-IP macros (one per byte lane).
-- That stack lane -- and the macros backing it -- is GONE:
-- components/sdram/sdram_ctrl.vhd self-initialises in hardware, so the
-- reset vector's SP now points into SDRAM instead of an on-chip
-- scratchpad (see targets/asic/gf180_j4mmu/boot_image_pkg.vhd). This
-- architecture is now IDENTICAL in substance to (boot_mem) itself -- a
-- pure read-only constant ROM, no vendor macro, no arbitration -- kept as
-- a distinct tech/gf180 architecture name only so
-- targets/asic/gf180_j4mmu/cpus_one_m0_gf180_arch.vhd, tb/cpus_xip_probe.
-- vhd and the LibreLane harden flow (targets/asic/gf180_j4mmu/librelane/
-- boot_mem/, run.sh macro=boot_mem, via boot_mem_top_gf180.vhd) do not
-- need to change their (boot_mem_gf180) binding. The harden run now
-- produces a tiny logic-only ROM die (no SRAM macro at all) -- see
-- docs/asic/gf180-vs-kianv-comparison.md for the measured area.
library ieee;
use ieee.std_logic_1164.all;
use ieee.numeric_std.all;
use work.cpu2j0_pack.all;
use work.boot_image_pkg.all;

architecture boot_mem_gf180 of bootram_infer is
  constant ROM_WORDS : integer := 2 ** (c_addr_width - 2); -- word-addressed, full window
  subtype word_t is std_logic_vector(31 downto 0);
  type rom_t is array (0 to ROM_WORDS - 1) of word_t;

  -- read-only constant ROM: identical to (boot_mem)'s init_rom.
  function init_rom return rom_t is
    variable m : rom_t := (others => (others => '0'));
  begin
    for i in 0 to ROM_WORDS - 1 loop
      if i < BOOT_DEPTH and i < ROM_WORDS then
        m(i) := BOOT_IMAGE(i);
      end if;
    end loop;
    return m;
  end function;

  constant rom : rom_t := init_rom;

  signal d_word : word_t := (others => '0');
  signal i_word : word_t := (others => '0');
  signal i_half : std_logic := '0';
begin
  -- synthesis translate_off
  assert BOOT_DEPTH <= ROM_WORDS
    report "boot_image_pkg BOOT_DEPTH exceeds boot_mem ROM depth; image truncated"
    severity warning;
  -- synthesis translate_on

  process(clk)
    variable di, ii : integer;
  begin
    if falling_edge(clk) then
      di := to_integer(unsigned(db_i.a(c_addr_width - 1 downto 2)));
      d_word <= rom(di);

      ii := to_integer(unsigned(ibus_i.a(c_addr_width - 1 downto 2)));
      i_word <= rom(ii);
      i_half <= ibus_i.a(1);
    end if;
  end process;

  db_o.d   <= d_word;
  db_o.ack <= db_i.en;

  ibus_o.d   <= i_word(31 downto 16) when i_half = '0' else i_word(15 downto 0);
  ibus_o.ack <= ibus_i.en;
end architecture;
