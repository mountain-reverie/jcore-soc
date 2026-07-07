-- spi_flash_fill: frame-fill engine for the iCESugar UP5K XIP page cache.
--
-- Adapted from the Fast-Read (0x0B) SPI mode-0 FSM in components/misc/spi_xip.vhd
-- (branch feat/icesugar-aic-spiboot, S_CMDADDR -> S_DUMMY -> S_DATA sequencing
-- and clk/2 bit-banger divider), with the line-cache/tag/direct-serve logic
-- (spi_xip's S_IDLE/S_WAIT_RD/S_LOOKUP/S_WRITEBACK/S_POSTACK and its
-- tag/valid/data RAMs) dropped entirely -- that was the superseded
-- direct-serve architecture. This engine instead streams one full 4 KB flash
-- page, word by word, straight into a victim frame's EBR write port on
-- command (`start`), for use by the spi_page_cache demand-paging MMIO block.
--
-- SPI flash pins here are the "d_*" side that ice_spi_io's SB_IO pad wrapper
-- is instantiated on top of -- ice_spi_io itself is instantiated at the cpus
-- level, not here (this entity never sees a physical pad).
--
-- Flash Fast-Read (0x0B) protocol, SPI mode 0 (CPOL=0/CPHA=0): drop cs_n,
-- shift out MSB-first on mosi (sampled by the flash on sck rising edges):
--   [8 bits: 0x0B] [24 bits: FLASH_BASE + (page & x"000")] [8 dummy bits]
-- then shift in 4096*8 bits MSB-first (byte 0 first) on miso, sampled by us
-- on sck rising edges. SH-2 is big-endian: the first flash byte occupies the
-- most-significant byte of the packed word (byte0 = bits 31..24).
--
-- fr_addr = frame(1:0) & word_index(9:0): frame*1024 + word_index, 0..4095.

library ieee;
use ieee.std_logic_1164.all;
use ieee.numeric_std.all;

entity spi_flash_fill is
  generic (
    FLASH_BASE : std_logic_vector(23 downto 0) := x"100000"); -- XIP image offset in flash
  port (
    clk : in std_logic; rst : in std_logic;
    -- command (from spi_page_cache MMIO)
    start : in  std_logic;                       -- pulse: begin a fill
    page  : in  std_logic_vector(7 downto 0);    -- flash page number (VA[19:12])
    frame : in  std_logic_vector(1 downto 0);    -- victim frame index (0..3)
    busy  : out std_logic;                       -- '1' during fill
    done  : out std_logic;                       -- level: '1' when the last fill completed, cleared on next start
    -- frame EBR write port (into spi_page_cache's frame RAM)
    fr_we   : out std_logic;
    fr_addr : out std_logic_vector(11 downto 0); -- {frame(1:0), word_off(9:0)} = 4096 words
    fr_data : out std_logic_vector(31 downto 0);
    -- config-flash pins (to ice_spi_io d_* side)
    d_cs_n : out std_logic; d_sck : out std_logic; d_mosi : out std_logic; d_miso : in std_logic);
end entity;

architecture rtl of spi_flash_fill is

  constant WORDS_PER_PAGE : natural := 1024;  -- 4096 bytes / 4

  type state_t is (S_IDLE, S_CMDADDR, S_DUMMY, S_DATA, S_DONE);
  signal state : state_t := S_IDLE;

  -- SPI bit-banger: each SPI bit takes two clk cycles (phase='0' -> drive
  -- mosi / sck low ("setup"), phase='1' -> sck high ("sample/edge")).
  signal phase     : std_logic := '0';
  signal bits_left : natural range 0 to WORDS_PER_PAGE * 32 := 0;
  signal shift_out : std_logic_vector(31 downto 0) := (others => '0'); -- CMD+ADDR shifter

  signal rx_byte     : std_logic_vector(7 downto 0) := (others => '0');
  signal bit_in_byte : natural range 0 to 7 := 0;
  signal byte_in_word : natural range 0 to 3 := 0;
  signal word_bytes   : std_logic_vector(31 downto 0) := (others => '0'); -- big-endian packing buffer

  signal word_idx : natural range 0 to WORDS_PER_PAGE - 1 := 0;
  signal frame_r  : std_logic_vector(1 downto 0) := (others => '0');

  signal cs_r   : std_logic := '1';
  signal sck_r  : std_logic := '0';
  signal mosi_r : std_logic := '0';

  signal busy_r : std_logic := '0';
  signal done_r : std_logic := '0';

  signal fr_we_r   : std_logic := '0';
  signal fr_addr_r : std_logic_vector(11 downto 0) := (others => '0');
  signal fr_data_r : std_logic_vector(31 downto 0) := (others => '0');

begin

  process (clk) is
    variable addr24 : std_logic_vector(23 downto 0);
  begin
    if rising_edge(clk) then
      if rst = '1' then
        state   <= S_IDLE;
        cs_r    <= '1';
        sck_r   <= '0';
        busy_r  <= '0';
        done_r  <= '0';
        fr_we_r <= '0';
      else
        -- default: fr_we is a single-cycle pulse
        fr_we_r <= '0';

        case state is
          ----------------------------------------------------------------
          when S_IDLE =>
            if start = '1' then
              addr24     := std_logic_vector(unsigned(FLASH_BASE) + (unsigned(page) & x"000"));
              shift_out  <= x"0B" & addr24;
              bits_left  <= 32;
              phase      <= '0';
              cs_r       <= '0';
              sck_r      <= '0';
              word_idx   <= 0;
              frame_r    <= frame;
              busy_r     <= '1';
              done_r     <= '0';
              state      <= S_CMDADDR;
            end if;

          ----------------------------------------------------------------
          when S_CMDADDR =>
            -- shift out 8 (cmd) + 24 (addr) = 32 bits MSB-first
            if phase = '0' then
              mosi_r <= shift_out(31);
              sck_r  <= '0';
              phase  <= '1';
            else
              sck_r     <= '1';
              shift_out <= shift_out(30 downto 0) & '0';
              bits_left <= bits_left - 1;
              phase     <= '0';
              if bits_left = 1 then
                bits_left <= 8;    -- 8 dummy clocks
                state     <= S_DUMMY;
              end if;
            end if;

          ----------------------------------------------------------------
          when S_DUMMY =>
            -- 8 dummy clocks; mosi is don't-care during dummy
            if phase = '0' then
              mosi_r <= '0';
              sck_r  <= '0';
              phase  <= '1';
            else
              sck_r     <= '1';
              bits_left <= bits_left - 1;
              phase     <= '0';
              if bits_left = 1 then
                bits_left    <= WORDS_PER_PAGE * 32;
                byte_in_word <= 0;
                bit_in_byte  <= 0;
                state        <= S_DATA;
              end if;
            end if;

          ----------------------------------------------------------------
          when S_DATA =>
            -- shift in 4096*8 bits MSB-first, packing 4 bytes/word
            -- big-endian (first byte -> bits 31..24), pulsing fr_we once
            -- per completed word.
            if phase = '0' then
              sck_r <= '0';
              phase <= '1';
            else
              sck_r     <= '1';
              rx_byte   <= rx_byte(6 downto 0) & d_miso;
              bits_left <= bits_left - 1;
              phase     <= '0';
              if bit_in_byte = 7 then
                case byte_in_word is
                  when 0 => word_bytes(31 downto 24) <= rx_byte(6 downto 0) & d_miso;
                  when 1 => word_bytes(23 downto 16) <= rx_byte(6 downto 0) & d_miso;
                  when 2 => word_bytes(15 downto 8)  <= rx_byte(6 downto 0) & d_miso;
                  when others => word_bytes(7 downto 0) <= rx_byte(6 downto 0) & d_miso;
                end case;
                bit_in_byte <= 0;
                if byte_in_word = 3 then
                  byte_in_word <= 0;
                  fr_we_r   <= '1';
                  fr_addr_r <= frame_r & std_logic_vector(to_unsigned(word_idx, 10));
                  fr_data_r <= word_bytes(31 downto 8) &
                               (rx_byte(6 downto 0) & d_miso); -- last byte of this word
                  if word_idx /= WORDS_PER_PAGE - 1 then
                    word_idx <= word_idx + 1;
                  end if;
                else
                  byte_in_word <= byte_in_word + 1;
                end if;
              else
                bit_in_byte <= bit_in_byte + 1;
              end if;
              if bits_left = 1 then
                cs_r   <= '1';
                sck_r  <= '0';
                state  <= S_DONE;
              end if;
            end if;

          ----------------------------------------------------------------
          when S_DONE =>
            busy_r <= '0';
            done_r <= '1';
            state  <= S_IDLE;

        end case;
      end if;
    end if;
  end process;

  busy    <= busy_r;
  done    <= done_r;
  fr_we   <= fr_we_r;
  fr_addr <= fr_addr_r;
  fr_data <= fr_data_r;

  d_cs_n <= cs_r;
  d_sck  <= sck_r;
  d_mosi <= mosi_r;

end architecture;
