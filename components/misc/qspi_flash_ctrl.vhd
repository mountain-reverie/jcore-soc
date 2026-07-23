-------------------------------------------------------------------------------
-- qspi_flash_ctrl.vhd
--
-- qspi_read_engine: Fast-Read (0x0B single-SPI / 0xEB Quad-I/O, selected by
-- the LANES generic) fill engine. Given a start address + a "start" pulse,
-- issues the command/address/dummy sequence and streams 32 bytes into a
-- 256-bit line register, MSB-first, mode-0, 2 clk cycles per SPI bit-time
-- (phase 0 = drive/sck-low setup, phase 1 = sck-high sample), CS held low
-- from the command byte through the end of the 32-byte data burst.
--
-- This is a direct start/done/line-register engine -- no bus interface.
-- Task 3 wraps this in the bus slave + double buffer.
--
-- Structure (phase divider, cs_r/sck_r registers) is reused from
-- components/misc/flash_boot_reader.vhd, generalized by the LANES generic
-- for both the single-SPI (0x0B) and quad-I/O (0xEB) datapaths; that file
-- is NOT modified.
--
-- LANES = 1: command 0x0B, 8 cmd bits + 24 addr bits shifted single-SPI on
--   io_o(0) (io_oe = "0001" while driving), 8 dummy clocks (fixed, per the
--   0x0B protocol -- DUMMY_CYCLES generic does not apply here), then 32
--   bytes read MSB-first on io_i(1) (io_oe = "0000" throughout dummy/data).
--
-- LANES = 4: command 0xEB, 8 cmd bits single-SPI on io_o(0) (io_oe="0001"),
--   then 24 addr bits QUAD on io_o(3 downto 0) (6 clocks, 4 bits/clock,
--   IO3=MSB of each nibble, io_oe="1111"), then DUMMY_CYCLES quad dummy
--   clocks (mode byte + dummy combined field; default 6, MUST match the
--   qspi_flash_model's QUAD_DUMMY_CYCLES), then 32 bytes read QUAD on
--   io_i(3 downto 0) (2 nibbles/byte, IO3=MSB nibble; io_oe="0000"
--   throughout dummy/data).
--
-- line_o byte<->bit mapping: line_o is 256 bits = 32 bytes, byte 0 (the
-- first byte read from the flash, at start_addr) in the MOST significant
-- byte lane and byte 31 (last byte read) in the LEAST significant byte
-- lane:
--   byte n (0..31) = line_o(255 - 8*n downto 248 - 8*n)
-- i.e. line_o(255 downto 248) = byte 0, line_o(7 downto 0) = byte 31.
-------------------------------------------------------------------------------

library ieee;
use ieee.std_logic_1164.all;
use ieee.numeric_std.all;

entity qspi_read_engine is
  generic (
    LANES        : natural := 4;   -- 1 => 0x0B single-SPI, 4 => 0xEB quad
    DUMMY_CYCLES : natural := 6);  -- quad mode-byte+dummy field (clocks);
                                    -- single mode always uses 8 dummy clocks
                                    -- per the 0x0B protocol, regardless of
                                    -- this generic
  port (
    clk : in std_logic;
    rst : in std_logic;

    -- command handshake
    start      : in  std_logic;                     -- pulse: begin fill
    start_addr : in  std_logic_vector(23 downto 0);
    busy       : out std_logic;
    done       : out std_logic;                     -- level, cleared on next start

    -- result
    line_o     : out std_logic_vector(255 downto 0); -- 32 bytes, see mapping above
    line_valid : out std_logic;                      -- level, cleared on next start

    -- flash pins
    cs_n  : out std_logic;
    sck   : out std_logic;
    io_o  : out std_logic_vector(3 downto 0);
    io_oe : out std_logic_vector(3 downto 0);
    io_i  : in  std_logic_vector(3 downto 0));
end entity;

architecture rtl of qspi_read_engine is

  function sel_cmd_byte(lanes : natural) return std_logic_vector is
  begin
    if lanes = 4 then
      return x"EB";
    else
      return x"0B";
    end if;
  end function;

  function sel_addr_clocks(lanes : natural) return natural is
  begin
    if lanes = 4 then
      return 6;  -- 24 addr bits, 4 bits/clock
    else
      return 24; -- 24 addr bits, 1 bit/clock
    end if;
  end function;

  function sel_dummy_clocks(lanes : natural; dummy_cycles : natural) return natural is
  begin
    if lanes = 4 then
      return dummy_cycles;
    else
      return 8; -- fixed per the 0x0B protocol
    end if;
  end function;

  function sel_data_units_per_byte(lanes : natural) return natural is
  begin
    if lanes = 4 then
      return 2; -- 2 nibbles/byte
    else
      return 8; -- 8 bits/byte
    end if;
  end function;

  constant CMD_BYTE            : std_logic_vector(7 downto 0) := sel_cmd_byte(LANES);
  constant ADDR_CLOCKS         : natural := sel_addr_clocks(LANES);
  constant DUMMY_CLOCKS        : natural := sel_dummy_clocks(LANES, DUMMY_CYCLES);
  constant DATA_UNITS_PER_BYTE : natural := sel_data_units_per_byte(LANES);

  type state_t is (S_IDLE, S_CMD, S_ADDR, S_DUMMY, S_DATA, S_DONE);
  signal state : state_t := S_IDLE;

  signal phase : std_logic := '0'; -- '0' = drive/setup (sck low), '1' = sample (sck high)

  signal cs_r  : std_logic := '1';
  signal sck_r : std_logic := '0';

  signal io_o_r  : std_logic_vector(3 downto 0) := "0000";
  signal io_oe_r : std_logic_vector(3 downto 0) := "0000";

  signal cmd_shift  : std_logic_vector(7 downto 0)  := (others => '0');
  signal addr_shift : std_logic_vector(23 downto 0) := (others => '0');

  signal bits_left : natural := 0; -- clocks remaining in current phase

  signal byte_shift  : std_logic_vector(7 downto 0) := (others => '0');
  signal units_left  : natural := 0; -- data units (bits or nibbles) remaining in current byte
  signal byte_idx     : natural range 0 to 31 := 0;

  signal line_r : std_logic_vector(255 downto 0) := (others => '0');

  signal busy_r        : std_logic := '0';
  signal done_r         : std_logic := '0';
  signal line_valid_r   : std_logic := '0';

begin

  process (clk) is
  begin
    if rising_edge(clk) then
      if rst = '1' then
        state        <= S_IDLE;
        cs_r         <= '1';
        sck_r        <= '0';
        io_oe_r      <= "0000";
        busy_r       <= '0';
        done_r       <= '0';
        line_valid_r <= '0';
      else
        case state is
          ------------------------------------------------------------------
          when S_IDLE =>
            if start = '1' then
              cmd_shift    <= CMD_BYTE;
              addr_shift   <= start_addr;
              bits_left    <= 8;
              phase        <= '0';
              cs_r         <= '0';
              sck_r        <= '0';
              io_oe_r      <= "0001"; -- cmd byte always single-SPI on IO0
              byte_idx     <= 0;
              busy_r       <= '1';
              done_r       <= '0';
              line_valid_r <= '0';
              state        <= S_CMD;
            end if;

          ------------------------------------------------------------------
          when S_CMD =>
            io_oe_r <= "0001";
            if phase = '0' then
              io_o_r(0) <= cmd_shift(7);
              sck_r     <= '0';
              phase     <= '1';
            else
              sck_r     <= '1';
              cmd_shift <= cmd_shift(6 downto 0) & '0';
              phase     <= '0';
              if bits_left = 1 then
                -- io_oe_r is left unchanged here (still "0001" from the cmd
                -- phase) so it stays stable through this cycle's rising
                -- edge (the last cmd bit's sample); S_ADDR unconditionally
                -- drives its own io_oe value starting next cycle, well
                -- before the first address bit's rising edge.
                bits_left <= ADDR_CLOCKS;
                state <= S_ADDR;
              else
                bits_left <= bits_left - 1;
              end if;
            end if;

          ------------------------------------------------------------------
          when S_ADDR =>
            if LANES = 4 then
              io_oe_r <= "1111";
              if phase = '0' then
                io_o_r     <= addr_shift(23 downto 20);
                sck_r      <= '0';
                phase      <= '1';
              else
                sck_r      <= '1';
                addr_shift <= addr_shift(19 downto 0) & "0000";
                phase      <= '0';
                if bits_left = 1 then
                  -- io_oe_r left unchanged (still "1111") through this
                  -- cycle's rising edge (last quad addr nibble's sample);
                  -- S_DUMMY unconditionally clears it starting next cycle.
                  bits_left <= DUMMY_CLOCKS;
                  state     <= S_DUMMY;
                else
                  bits_left <= bits_left - 1;
                end if;
              end if;
            else
              io_oe_r <= "0001";
              if phase = '0' then
                io_o_r(0)  <= addr_shift(23);
                sck_r      <= '0';
                phase      <= '1';
              else
                sck_r      <= '1';
                addr_shift <= addr_shift(22 downto 0) & '0';
                phase      <= '0';
                if bits_left = 1 then
                  -- io_oe_r left unchanged (still "0001") through this
                  -- cycle's rising edge (last single addr bit's sample);
                  -- S_DUMMY unconditionally clears it starting next cycle.
                  bits_left <= DUMMY_CLOCKS;
                  state     <= S_DUMMY;
                else
                  bits_left <= bits_left - 1;
                end if;
              end if;
            end if;

          ------------------------------------------------------------------
          when S_DUMMY =>
            io_oe_r <= "0000";
            if phase = '0' then
              sck_r <= '0';
              phase <= '1';
            else
              sck_r <= '1';
              phase <= '0';
              if bits_left = 1 then
                byte_idx    <= 0;
                byte_shift  <= (others => '0');
                units_left  <= DATA_UNITS_PER_BYTE;
                state       <= S_DATA;
              else
                bits_left <= bits_left - 1;
              end if;
            end if;

          ------------------------------------------------------------------
          when S_DATA =>
            io_oe_r <= "0000";
            if phase = '0' then
              sck_r <= '0';
              phase <= '1';
            else
              sck_r <= '1';
              phase <= '0';

              if LANES = 4 then
                byte_shift <= byte_shift(3 downto 0) & io_i(3 downto 0);
              else
                byte_shift <= byte_shift(6 downto 0) & io_i(1);
              end if;

              if units_left = 1 then
                units_left <= DATA_UNITS_PER_BYTE;
                if LANES = 4 then
                  line_r(255 - 8*byte_idx downto 248 - 8*byte_idx) <=
                    byte_shift(3 downto 0) & io_i(3 downto 0);
                else
                  line_r(255 - 8*byte_idx downto 248 - 8*byte_idx) <=
                    byte_shift(6 downto 0) & io_i(1);
                end if;
                if byte_idx = 31 then
                  state <= S_DONE;
                  cs_r  <= '1';
                  sck_r <= '0';
                else
                  byte_idx <= byte_idx + 1;
                end if;
              else
                units_left <= units_left - 1;
              end if;
            end if;

          ------------------------------------------------------------------
          when S_DONE =>
            busy_r       <= '0';
            done_r       <= '1';
            line_valid_r <= '1';
            io_oe_r      <= "0000";
            state        <= S_IDLE;

        end case;
      end if;
    end if;
  end process;

  busy       <= busy_r;
  done       <= done_r;
  line_valid <= line_valid_r;
  line_o     <= line_r;

  cs_n  <= cs_r;
  sck   <= sck_r;
  io_o  <= io_o_r;
  io_oe <= io_oe_r;

end architecture;
