library ieee;
use ieee.std_logic_1164.all;
use work.cpu2j0_pack.all;

package spi_page_cache_pack is

  constant PC_WIN_BASE   : std_logic_vector(31 downto 0) := x"40000000"; -- flash window
  constant PC_WIN_TOPBIT : natural := 20;  -- window = a(31 downto 20)=x"400"
  constant PC_FRAME_BASE : std_logic_vector(31 downto 0) := x"40100000"; -- CPU-addressable frames
  constant PC_MMIO_BASE  : std_logic_vector(31 downto 0) := x"ABCD0400";
  constant PC_NFRAMES    : natural := 4;
  constant PC_PAGE_BITS  : natural := 12;                 -- 4 KB pages

  subtype  pc_pageno_t is std_logic_vector(7 downto 0);   -- VA(19 downto 12)

  type pc_tag_t is record
    valid : std_logic;
    page  : pc_pageno_t;
  end record;

  type pc_tag_array_t is array(0 to PC_NFRAMES-1) of pc_tag_t;

  constant PC_TAG_RESET : pc_tag_t := (valid => '0', page => (others => '0'));

  -- MMIO word offsets: TAG0..3 = 0x00..0x0C, FAULT_VA=0x10, STATUS=0x14,
  --   FILL_CMD=0x18  (write {frame[9:8], page[7:0], start=bit0-strobe}; starts a
  --                   spi_flash_fill of the flash page into the victim frame),
  --   FILL_STATUS=0x1C (read: bit0=busy, bit1=done).
  -- Fill is HW-assisted (spi_flash_fill engine, Task 1) -- NOT a SW byte-copy.

end package spi_page_cache_pack;

package body spi_page_cache_pack is

end spi_page_cache_pack;
