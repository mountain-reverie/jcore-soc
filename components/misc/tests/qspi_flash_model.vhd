-------------------------------------------------------------------------------
-- qspi_flash_model.vhd
--
-- Behavioral (NOT synthesizable-required) model of a quad SPI-NOR flash
-- device, for use as a testbench dependency by QSPI controller tests.
--
-- Answers two commands:
--   0x0B  Fast-Read (single-SPI): 8 cmd bits (IO0) + 24 addr bits (IO0)
--         + 8 dummy clocks, then data out MSB-first on IO1 (MISO) only,
--         one bit per SCK, auto-incrementing address while CS stays low.
--   0xEB  Quad-I/O Fast-Read: 8 cmd bits single (IO0) + 24 addr bits QUAD
--         (IO0-3, 4 bits/clock MSB-first => 6 clocks) + a combined
--         mode-byte/dummy field of QUAD_DUMMY_CYCLES clocks (this model
--         uses 6, i.e. one nibble is the "mode byte" and the rest is
--         dummy -- callers/controllers MUST match this exact count),
--         then data out QUAD on IO0-3 (4 bits/clock, 1 byte per 2 clocks),
--         MSB-first, auto-incrementing.
--
-- Deterministic data pattern (so any testbench can predict any byte
-- without needing to load/share memory contents):
--     byte(addr) = addr(7 downto 0) xor addr(15 downto 8)
-- where addr is the 24-bit flash byte address. addr(23 downto 16) does
-- not affect the byte value.
--
-- IO triplet convention (avoids needing a resolved inout inside this
-- model -- the enclosing testbench/top is responsible for resolving the
-- shared IO0-3 bus between the controller and this model):
--   io_i  : in  std_logic_vector(3 downto 0)
--       The 4 IO lines AS DRIVEN BY THE CONTROLLER (only meaningful on
--       lines the controller is actually driving, see io_oe).
--   io_oe : in  std_logic_vector(3 downto 0)
--       The controller's per-line output-enable, so this model knows
--       who is driving each line at any given time.
--   io_o  : out std_logic_vector(3 downto 0)
--       The lines THIS MODEL drives. Only driven during the data phase:
--       IO1 only in single (0x0B) mode, IO0-3 in quad (0xEB) mode. All
--       other times this model drives 'Z' (high-impedance) so the
--       resolved bus reflects only the controller.
--
-- PROTOCOL ASSERTIONS: this model asserts (severity failure) if, during
-- the command/address phase, the controller is not driving the lines it
-- is supposed to drive (io_oe mismatch), or if, during the data phase,
-- the controller fails to tristate the line(s) this model is driving
-- (io_oe(1) /= '0' in single mode, io_oe(3 downto 0) /= "0000" in quad
-- mode). This is the point of the model: it makes controller OE bugs
-- fail loudly instead of silently corrupting data.
--
-- Sampling/driving convention: SPI mode 0 (CPOL=0, CPHA=0), MSB-first.
-- This model samples io_i on the RISING edge of sck and updates io_o
-- (for the following bit) on the FALLING edge of sck, which is the
-- standard mode-0 slave behavior. CS is held low across the whole
-- burst; the model resets its internal state machine on the falling
-- edge of cs_n and stops driving on the rising edge of cs_n.
-------------------------------------------------------------------------------

library ieee;
use ieee.std_logic_1164.all;
use ieee.numeric_std.all;

entity qspi_flash_model is
  generic (
    -- Optional preload window (Task 4, QSPI XIP cosim): when PRELOAD_EN='1',
    -- byte(addr) for addr in [0,32) comes from PRELOAD (256 bits = 32 bytes,
    -- byte 0 in bits 255:248 ... byte 31 in bits 7:0 -- the SAME MSB-first
    -- byte-lane mapping qspi_flash_ctrl.vhd uses for its 32-byte line_o, so
    -- a payload image built for that mapping drops in unchanged). addr>=32
    -- (and the DEFAULT PRELOAD_EN='0') always falls back to the synthetic
    -- byte(addr)=addr(7:0) xor addr(15:8) pattern documented above, so every
    -- existing caller (qspi_sim.sh) is byte-for-byte unaffected.
    PRELOAD_EN : std_logic := '0';
    PRELOAD    : std_logic_vector(255 downto 0) := (others => '0'));
  port (
    cs_n  : in  std_logic;
    sck   : in  std_logic;
    io_i  : in  std_logic_vector(3 downto 0);
    io_oe : in  std_logic_vector(3 downto 0);
    io_o  : out std_logic_vector(3 downto 0)
  );
end entity;

architecture behavioral of qspi_flash_model is

  -- byte(addr): PRELOAD window (Task 4 XIP cosim) when enabled and in
  -- range, else the standard synthetic pattern.
  function flash_byte(addr : unsigned(23 downto 0)) return std_logic_vector is
    variable n : integer;
  begin
    n := to_integer(addr(23 downto 0));
    if PRELOAD_EN = '1' and n < 32 then
      return PRELOAD(255 - 8*n downto 248 - 8*n);
    else
      return std_logic_vector(addr(7 downto 0) xor addr(15 downto 8));
    end if;
  end function;

  constant CMD_FAST_READ      : std_logic_vector(7 downto 0) := x"0B";
  constant CMD_QUAD_IO_READ   : std_logic_vector(7 downto 0) := x"EB";

  constant SINGLE_DUMMY_CYCLES : integer := 8;
  -- Combined mode-byte + dummy field for 0xEB, expressed in quad clocks
  -- (4 bits/clock). Documented above: callers/controllers MUST match
  -- this exact count.
  constant QUAD_DUMMY_CYCLES   : integer := 6;

  type phase_t is (PH_IDLE, PH_CMD, PH_ADDR, PH_DUMMY, PH_DATA);

  type mode_t is (MODE_NONE, MODE_SINGLE, MODE_QUAD);

begin

  process (cs_n, sck)
    variable phase      : phase_t := PH_IDLE;
    variable mode       : mode_t  := MODE_NONE;
    variable bit_cnt     : integer := 0;   -- bits (or quad-nibbles) counted in current phase
    variable cmd_shift   : std_logic_vector(7 downto 0)  := (others => '0');
    variable addr_shift  : std_logic_vector(23 downto 0) := (others => '0');
    variable addr        : unsigned(23 downto 0) := (others => '0');
    variable data_byte   : std_logic_vector(7 downto 0) := (others => '0');
    variable data_bitpos : integer := 7; -- next bit (single) / next nibble idx (quad)
  begin

    if cs_n = '1' then
      -- Not selected: release the bus, reset state so a new falling
      -- edge starts a clean burst.
      io_o  <= "ZZZZ";
      phase := PH_IDLE;
      mode  := MODE_NONE;
      bit_cnt := 0;

    elsif falling_edge(cs_n) then
      phase := PH_CMD;
      mode  := MODE_NONE;
      bit_cnt := 0;
      cmd_shift := (others => '0');
      io_o <= "ZZZZ";

    elsif rising_edge(sck) and cs_n = '0' then
      case phase is

        when PH_CMD =>
          assert io_oe(0) = '1'
            report "qspi_flash_model: protocol violation - controller not" &
                   " driving IO0 during command phase (io_oe(0)='0')"
            severity failure;

          cmd_shift := cmd_shift(6 downto 0) & io_i(0);
          bit_cnt := bit_cnt + 1;

          if bit_cnt = 8 then
            bit_cnt := 0;
            if cmd_shift = CMD_FAST_READ then
              mode := MODE_SINGLE;
            elsif cmd_shift = CMD_QUAD_IO_READ then
              mode := MODE_QUAD;
            else
              mode := MODE_NONE;
            end if;
            phase := PH_ADDR;
            addr_shift := (others => '0');
          end if;

        when PH_ADDR =>
          if mode = MODE_SINGLE then
            assert io_oe(0) = '1'
              report "qspi_flash_model: protocol violation - controller not" &
                     " driving IO0 during address phase (single, io_oe(0)='0')"
              severity failure;

            addr_shift := addr_shift(22 downto 0) & io_i(0);
            bit_cnt := bit_cnt + 1;
            if bit_cnt = 24 then
              addr := unsigned(addr_shift);
              bit_cnt := 0;
              phase := PH_DUMMY;
            end if;

          elsif mode = MODE_QUAD then
            assert io_oe(3 downto 0) = "1111"
              report "qspi_flash_model: protocol violation - controller not" &
                     " driving IO0-3 during address phase (quad, io_oe=" &
                     " expected 1111)"
              severity failure;

            addr_shift := addr_shift(19 downto 0) & io_i(3) & io_i(2) & io_i(1) & io_i(0);
            bit_cnt := bit_cnt + 1;
            if bit_cnt = 6 then
              addr := unsigned(addr_shift);
              bit_cnt := 0;
              phase := PH_DUMMY;
            end if;

          else
            -- Unknown command: just count 24 bits on IO0 and ignore.
            bit_cnt := bit_cnt + 1;
            if bit_cnt = 24 then
              bit_cnt := 0;
              phase := PH_DUMMY;
            end if;
          end if;

        when PH_DUMMY =>
          -- Controller still drives command/mode info during (part of)
          -- this window on quad mode-byte convention; we do not assert
          -- OE here since the mode byte/dummy driver behavior is
          -- implementation-defined beyond "controller drives, model
          -- doesn't". We simply count cycles.
          bit_cnt := bit_cnt + 1;

          if mode = MODE_SINGLE and bit_cnt = SINGLE_DUMMY_CYCLES then
            bit_cnt := 0;
            phase := PH_DATA;
            data_byte := flash_byte(addr);
            data_bitpos := 7;
          elsif mode = MODE_QUAD and bit_cnt = QUAD_DUMMY_CYCLES then
            bit_cnt := 0;
            phase := PH_DATA;
            data_byte := flash_byte(addr);
            data_bitpos := 1; -- nibble index: 1 = MSB nibble, 0 = LSB nibble
          elsif mode = MODE_NONE then
            -- unknown command: never produce data
            null;
          end if;

        when PH_DATA =>
          if mode = MODE_SINGLE then
            assert io_oe(1) = '0'
              report "qspi_flash_model: protocol violation - controller" &
                     " must tristate IO1 during data phase (single," &
                     " io_oe(1)/='0')"
              severity failure;
          elsif mode = MODE_QUAD then
            assert io_oe(3 downto 0) = "0000"
              report "qspi_flash_model: protocol violation - controller" &
                     " must tristate IO0-3 during data phase (quad," &
                     " io_oe/='0000')"
              severity failure;
          end if;

          -- Advance to the NEXT bit/nibble; the bit/nibble currently
          -- being sampled here was already driven on the prior falling
          -- edge (see falling_edge(sck) process branch below).
          if mode = MODE_SINGLE then
            if data_bitpos = 0 then
              addr := addr + 1;
              data_byte := flash_byte(addr);
              data_bitpos := 7;
            else
              data_bitpos := data_bitpos - 1;
            end if;
          elsif mode = MODE_QUAD then
            if data_bitpos = 0 then
              addr := addr + 1;
              data_byte := flash_byte(addr);
              data_bitpos := 1;
            else
              data_bitpos := data_bitpos - 1;
            end if;
          end if;

        when others =>
          null;
      end case;

    elsif falling_edge(sck) and cs_n = '0' then
      -- Drive the next bit/nibble ahead of the next rising edge, per
      -- mode-0 slave convention.
      case phase is
        when PH_DATA =>
          if mode = MODE_SINGLE then
            io_o    <= "ZZ" & data_byte(data_bitpos) & 'Z';
          elsif mode = MODE_QUAD then
            if data_bitpos = 1 then
              io_o <= data_byte(7 downto 4);
            else
              io_o <= data_byte(3 downto 0);
            end if;
          else
            io_o <= "ZZZZ";
          end if;
        when others =>
          io_o <= "ZZZZ";
      end case;
    end if;

  end process;

end architecture;
