-- flash_boot_reader: boot-time Fast-Read (0x0B) engine that streams a fixed
-- number of words from the UP5K config SPI flash directly into on-chip
-- SPRAM, for use before the CPU/bus is up (early boot payload loader).
--
-- Adapted from components/misc/spi_flash_fill.vhd (branch
-- feat/icesugar-spi-page-cache): same Fast-Read SPI mode-0 bit-banged FSM
-- (S_IDLE -> S_CMDADDR -> S_DUMMY -> S_DATA -> S_DONE, clk/2 phase divider,
-- MSB-first, CS held low across the whole burst since Fast-Read
-- auto-increments the flash's internal address while CS stays low).
--
-- Generalized vs spi_flash_fill for a free-running full-payload stream
-- rather than a single 4KB page-cache fill:
--   * PAYLOAD_WORDS is a generic (not a fixed 1024-word page), so the word
--     counter/index width is sized for values > 1024 (natural, unconstrained
--     range up here; synthesizes to enough bits for the generic's value).
--   * FLASH_BASE is used directly as the 24-bit read address (no page/frame
--     input -- this engine always starts its stream at FLASH_BASE).
--   * Output write port is the SPRAM write-port shape (sp_en/sp_we/sp_a/
--     sp_dw) instead of spi_flash_fill's frame-EBR port (fr_we/fr_addr/
--     fr_data), and sp_a is 15 bits (word address, bit 14 used; SPRAM's
--     external a(16 downto 2) bank/word address) vs spi_flash_fill's 12-bit
--     {frame,word_off} address.
--   * No page/frame inputs -- start is the sole command pulse.
--
-- SPI flash pins are the "d_*" side that ice_spi_io's SB_IO pad wrapper
-- sits on top of (ice_spi_io is instantiated elsewhere, at board top level).

library ieee;
use ieee.std_logic_1164.all;
use ieee.numeric_std.all;

entity flash_boot_reader is
  generic (
    FLASH_BASE    : std_logic_vector(23 downto 0) := x"100000";
    PAYLOAD_WORDS : natural := 8192);
  port (
    clk : in std_logic; rst : in std_logic;

    -- command
    start : in  std_logic;  -- pulse: begin streaming FLASH_BASE.. into SPRAM
    busy  : out std_logic;  -- '1' while a stream is in progress
    done  : out std_logic;  -- level: '1' once the stream completed, cleared on next start

    -- SPRAM write port (spram_128k shape: en/we/a(16 downto 2)/dw)
    sp_en : out std_logic;
    sp_we : out std_logic_vector(3 downto 0);
    sp_a  : out std_logic_vector(16 downto 2);
    sp_dw : out std_logic_vector(31 downto 0);

    -- config-flash pins (to ice_spi_io d_* side)
    d_cs_n : out std_logic; d_sck : out std_logic; d_mosi : out std_logic; d_miso : in std_logic);
end entity;

architecture rtl of flash_boot_reader is

  type state_t is (S_IDLE, S_CMDADDR, S_DUMMY, S_DATA, S_DONE);
  signal state : state_t := S_IDLE;

  -- SPI bit-banger: each SPI bit takes two clk cycles (phase='0' -> drive
  -- mosi / sck low ("setup"), phase='1' -> sck high ("sample/edge")).
  signal phase     : std_logic := '0';
  signal bits_left : natural range 0 to PAYLOAD_WORDS * 32 := 0;
  signal shift_out : std_logic_vector(31 downto 0) := (others => '0'); -- CMD+ADDR shifter

  signal rx_byte      : std_logic_vector(7 downto 0) := (others => '0');
  signal bit_in_byte  : natural range 0 to 7 := 0;
  signal byte_in_word : natural range 0 to 3 := 0;
  signal word_bytes   : std_logic_vector(31 downto 0) := (others => '0'); -- big-endian packing buffer

  signal word_idx : natural range 0 to PAYLOAD_WORDS - 1 := 0;

  signal cs_r   : std_logic := '1';
  signal sck_r  : std_logic := '0';
  signal mosi_r : std_logic := '0';

  signal busy_r : std_logic := '0';
  signal done_r : std_logic := '0';

  signal sp_en_r : std_logic := '0';
  signal sp_a_r  : std_logic_vector(16 downto 2) := (others => '0');
  signal sp_dw_r : std_logic_vector(31 downto 0) := (others => '0');

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
        sp_en_r <= '0';
      else
        -- default: sp_en is a single-cycle write-strobe pulse
        sp_en_r <= '0';

        case state is
          ----------------------------------------------------------------
          when S_IDLE =>
            if start = '1' then
              addr24    := FLASH_BASE;
              shift_out <= x"0B" & addr24;
              bits_left <= 32;
              phase     <= '0';
              cs_r      <= '0';
              sck_r     <= '0';
              word_idx  <= 0;
              busy_r    <= '1';
              done_r    <= '0';
              state     <= S_CMDADDR;
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
                bits_left    <= PAYLOAD_WORDS * 32;
                byte_in_word <= 0;
                bit_in_byte  <= 0;
                state        <= S_DATA;
              end if;
            end if;

          ----------------------------------------------------------------
          when S_DATA =>
            -- shift in PAYLOAD_WORDS*32 bits MSB-first, packing 4 bytes/word
            -- big-endian (first byte -> bits 31..24), pulsing sp_en once per
            -- completed word.
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
                  sp_en_r <= '1';
                  sp_a_r  <= std_logic_vector(to_unsigned(word_idx, 15));
                  sp_dw_r <= word_bytes(31 downto 8) &
                             (rx_byte(6 downto 0) & d_miso); -- last byte of this word
                  if word_idx /= PAYLOAD_WORDS - 1 then
                    word_idx <= word_idx + 1;
                  end if;
                else
                  byte_in_word <= byte_in_word + 1;
                end if;
              else
                bit_in_byte <= bit_in_byte + 1;
              end if;
              if bits_left = 1 then
                cs_r  <= '1';
                sck_r <= '0';
                state <= S_DONE;
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

  busy <= busy_r;
  done <= done_r;

  sp_en <= sp_en_r;
  sp_we <= "1111" when sp_en_r = '1' else "0000";
  sp_a  <= sp_a_r;
  sp_dw <= sp_dw_r;

  d_cs_n <= cs_r;
  d_sck  <= sck_r;
  d_mosi <= mosi_r;

end architecture;
