library ieee;
use ieee.std_logic_1164.all;
use ieee.numeric_std.all;
use work.cpu2j0_pack.all;
use work.boot_image_pkg.all;

-- Silicon-correct boot memory: an SRAM macro powers up with undefined
-- contents and cannot hold a reset vector, so this is a PURE READ-ONLY
-- constant ROM holding the SH-2 reset vector (BOOT_IMAGE words). A
-- registered read with NO write port, so this synthesizes to gates (a
-- logic ROM), not a RAM macro. Writes anywhere in this window are
-- silently ignored.
--
-- SCRATCHPAD REMOVAL (boot-memory refinement): this architecture used to
-- split the address space in half via a(c_addr_width-1) -- a low
-- read-only ROM lane plus a high 2 KB writable stack-SRAM lane, used for
-- the SH-2 boot-time stack before SDRAM was initialised. That stack lane
-- is GONE: components/sdram/sdram_ctrl.vhd self-initialises in hardware
-- (its FSM runs the full SDRAM init automatically from reset, serving
-- accesses only once idle -- the first SDRAM access simply stalls until
-- init completes), so the reset vector's SP now points directly into
-- SDRAM (see targets/asic/gf180_j4mmu/boot_image_pkg.vhd) and no on-chip
-- writable scratchpad is needed at all. The whole boot_mem window is now
-- read-only ROM.
architecture boot_mem of bootram_infer is
  constant ROM_WORDS : integer := 2 ** (c_addr_width - 2); -- word-addressed, full window
  subtype word_t is std_logic_vector(31 downto 0);
  type rom_t is array (0 to ROM_WORDS - 1) of word_t;

  -- read-only constant ROM: filled from the boot image, zero past it.
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

  -- Data port (read-only) and instruction port (read-only), both on
  -- falling edge so registered output is valid the same cycle ack=en is
  -- asserted, matching (inferred)'s / memory_fpga's 0-wait contract.
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

  -- Timing contract identical to (inferred): ack is combinational = en,
  -- while d_word/i_word are registered from the falling edge. Writes are
  -- silently ignored (no write port at all).
  db_o.d   <= d_word;
  db_o.ack <= db_i.en;

  -- big-endian halfword select: a(1)='0' -> high half (bits 31:16).
  ibus_o.d   <= i_word(31 downto 16) when i_half = '0' else i_word(15 downto 0);
  ibus_o.ack <= ibus_i.en;
end architecture;
