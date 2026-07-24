library ieee;
use ieee.std_logic_1164.all;
use ieee.numeric_std.all;
use work.cpu2j0_pack.all;
use work.boot_image_pkg.all;

-- Silicon-correct boot memory: an SRAM macro powers up with undefined
-- contents and cannot hold a reset vector, so the boot region is split by
-- address bit a(11):
--   0x000-0x7FF (is_rom='1'): read-only constant ROM holding the SH-2 reset
--     vector (BOOT_IMAGE low words). A registered read with NO write port,
--     so this synthesizes to gates (a logic ROM), not a RAM macro. Writes
--     to this window are silently ignored.
--   0x800-0xFFF (is_rom='0'): a small writable array (2 KB with
--     c_addr_width=12) for the boot-time stack, byte-lane writable exactly
--     like (inferred)'s data port.
-- Both ports mirror (inferred)'s 0-wait falling-edge contract: registered
-- read data is valid the cycle db_o.ack/ibus_o.ack (combinational = en) is
-- sampled at the following rising edge.
architecture boot_mem of bootram_infer is
  constant ROM_WORDS  : integer := 2 ** (c_addr_width - 3); -- low half, word-addressed
  constant SRAM_WORDS : integer := 2 ** (c_addr_width - 3); -- high half, word-addressed
  subtype word_t is std_logic_vector(31 downto 0);
  type rom_t  is array (0 to ROM_WORDS - 1)  of word_t;
  type sram_t is array (0 to SRAM_WORDS - 1) of word_t;

  -- read-only constant ROM lane: filled from the boot image, zero past it.
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

  signal sram : sram_t := (others => (others => '0'));

  signal d_word    : word_t := (others => '0');
  signal i_word    : word_t := (others => '0');
  signal i_half    : std_logic := '0';
begin
  -- synthesis translate_off
  assert BOOT_DEPTH <= ROM_WORDS
    report "boot_image_pkg BOOT_DEPTH exceeds boot_mem ROM lane depth; image truncated"
    severity warning;
  -- synthesis translate_on

  -- Data port (read/write) and instruction port (read-only), both on
  -- falling edge so registered output is valid the same cycle ack=en is
  -- asserted, matching (inferred)'s / memory_fpga's 0-wait contract.
  process(clk)
    variable di : integer;
  begin
    if falling_edge(clk) then
      -- data port: a(c_addr_width-1) selects ROM (high='0', i.e. low half
      -- of the address space) vs SRAM (high half); index within each lane
      -- uses the remaining word bits.
      di := to_integer(unsigned(db_i.a(c_addr_width - 2 downto 2)));
      if db_i.a(c_addr_width - 1) = '0' then
        -- ROM window: writes ignored; read the constant lane.
        d_word <= rom(di);
      else
        -- SRAM window: byte-lane writes, READ_FIRST (matches (inferred)).
        if db_i.en = '1' and db_i.wr = '1' then
          if db_i.we(0) = '1' then sram(di)(7 downto 0)   <= db_i.d(7 downto 0);   end if;
          if db_i.we(1) = '1' then sram(di)(15 downto 8)  <= db_i.d(15 downto 8);  end if;
          if db_i.we(2) = '1' then sram(di)(23 downto 16) <= db_i.d(23 downto 16); end if;
          if db_i.we(3) = '1' then sram(di)(31 downto 24) <= db_i.d(31 downto 24); end if;
        end if;
        d_word <= sram(di);
      end if;

      -- instruction port: read-only, same half-select convention.
      if ibus_i.a(c_addr_width - 1) = '0' then
        i_word <= rom(to_integer(unsigned(ibus_i.a(c_addr_width - 2 downto 2))));
      else
        i_word <= sram(to_integer(unsigned(ibus_i.a(c_addr_width - 2 downto 2))));
      end if;
      i_half <= ibus_i.a(1);
    end if;
  end process;

  -- Timing contract identical to (inferred): ack is combinational = en,
  -- while d_word/i_word are registered from the falling edge.
  db_o.d   <= d_word;
  db_o.ack <= db_i.en;

  -- big-endian halfword select: a(1)='0' -> high half (bits 31:16).
  ibus_o.d   <= i_word(31 downto 16) when i_half = '0' else i_word(15 downto 0);
  ibus_o.ack <= ibus_i.en;
end architecture;
