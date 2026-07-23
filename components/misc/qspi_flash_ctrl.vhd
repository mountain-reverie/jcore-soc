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

-------------------------------------------------------------------------------
-- qspi_flash_ctrl
--
-- Task 3: memory-mapped, read-only bus SLAVE wrapping qspi_read_engine
-- (above) in a double 32-byte ping-pong line buffer with sequential
-- prefetch and multi-cycle (deferred) ack.
--
-- Bus protocol (jcore convention, cf. components/misc/gpio2.vhd /
-- components/sdram/sdram_ctrl.vhd): db_i.en is held (along with a/rd/wr/
-- we/d) by the requester until db_o.ack is seen; ack is asserted for
-- exactly the one cycle the response is valid. Reads: db_o.d valid the
-- same cycle as ack. Writes to this region: ack, no effect (read-only).
--
-- db_o.d ENDIANNESS: this bus is big-endian in the sense that, for a
-- given aligned word, the FIRST byte read from the flash (the lowest
-- flash byte address) lands in the MOST significant byte of db_o.d:
--   db_o.d(31 downto 24) = byte(word_addr + 0)
--   db_o.d(23 downto 16) = byte(word_addr + 1)
--   db_o.d(15 downto  8) = byte(word_addr + 2)
--   db_o.d( 7 downto  0) = byte(word_addr + 3)
-- This mirrors qspi_read_engine's line_o mapping directly (byte n at
-- line_o(255-8n downto 248-8n)): unpack_words() below simply slices
-- line_o into 8 32-bit words with no byte reversal.
--
-- Line buffers: two records {tag (line-aligned 24-bit flash byte addr),
-- valid, 8x32-bit words}. On a bus read: compute the flash byte address
-- (db_i.a - FLASH_BASE), line-align it (& ~31), and take the word index
-- from addr(4 downto 2) (8 words/line). HIT (either buffer valid and
-- tag-matched) -> return the word and ack the SAME cycle. MISS -> latch
-- the request (aligned addr + word index), pick a victim buffer
-- (round-robin via last_filled), and start an engine fill; ack is
-- DEFERRED until the engine's `done`, at which point the buffer is
-- committed and the response (d + ack) is produced.
--
-- Sequential prefetch: on a HIT whose word index >= PREFETCH_THRESHOLD
-- (6 of 0..7, i.e. the last two words of the line), if the engine is
-- idle and the OTHER buffer does not already hold the next sequential
-- line (aligned+32), a run-ahead fill of that next line is started into
-- the other buffer, in the background (no ack dependency). Demand
-- (miss) fills always have priority: a prefetch is only started when
-- there is no demand fill pending; if a miss occurs while a prefetch is
-- in flight, the demand fill is deferred until the engine frees up
-- (single engine, no abort/preemption).
-------------------------------------------------------------------------------

library ieee;
use ieee.std_logic_1164.all;
use ieee.numeric_std.all;
use work.cpu2j0_pack.all;

-- BUS-PROTOCOL INVARIANT (deferred / multi-cycle ack): while this slave defers
-- db_o.ack='0' during an in-flight flash fill, the master MUST hold db_i.a (and
-- db_i.rd/en) steady until the ack cycle. This is the standard jcore data-bus
-- convention for a multi-cycle-ack slave (the same assumption components/sdram/
-- sdram_ctrl.vhd relies on): a transaction address is stable across its whole
-- ack window. If a master ever presented a different address mid-deferred-ack,
-- the same-cycle new-request-vs-fill-completion ordering could latch the wrong
-- d_r/ack_r -- not reachable under the jcore bus, but stated here explicitly.
entity qspi_flash_ctrl is
  generic (
    LANES        : natural := 4;
    DUMMY_CYCLES : natural := 6;
    FLASH_BASE   : std_logic_vector(31 downto 0) := (others => '0'));  -- region base address, for decode
  port (
    clk : in std_logic;
    rst : in std_logic;

    db_i  : in  cpu_data_o_t;
    bst   : in  std_logic := '0';               -- burst flag (mirrors sdram_ctrl's req/bst/resp/ack_r
                                                 -- contract); default '0' keeps SP1's single-word-only
                                                 -- port maps (no bst/ack_r binding) unchanged
    db_o  : out cpu_data_i_t;
    ack_r : out std_logic;                     -- read-ack strobe, asserted alongside db_o.ack for
                                                -- every READ beat (single or burst), per sdram_ctrl

    fl_cs_n  : out std_logic;
    fl_sck   : out std_logic;
    fl_io_o  : out std_logic_vector(3 downto 0);
    fl_io_oe : out std_logic_vector(3 downto 0);
    fl_io_i  : in  std_logic_vector(3 downto 0));
end entity;

architecture rtl of qspi_flash_ctrl is

  constant FLASH_SIZE          : natural := 16#1000000#; -- 16 MiB (24-bit flash addr space)
  constant PREFETCH_THRESHOLD  : natural := 6;            -- word idx 6,7 of 0..7 trigger run-ahead

  type words_t is array (0 to 7) of std_logic_vector(31 downto 0);

  type lbuf_t is record
    tag   : std_logic_vector(23 downto 0);
    valid : std_logic;
    words : words_t;
  end record;

  constant LBUF_RESET : lbuf_t := (
    tag   => (others => '0'),
    valid => '0',
    words => (others => (others => '0')));

  signal buf0, buf1 : lbuf_t := LBUF_RESET;

  -- qspi_read_engine wires
  signal eng_start      : std_logic := '0';
  signal eng_start_addr : std_logic_vector(23 downto 0) := (others => '0');
  signal eng_busy       : std_logic;
  signal eng_done       : std_logic;
  signal eng_line_valid : std_logic;
  signal eng_line_o     : std_logic_vector(255 downto 0);

  -- in-flight fill bookkeeping (single engine: at most one fill at a time)
  signal fill_kind   : natural range 0 to 2 := 0; -- 0 = none, 1 = demand, 2 = prefetch
  signal fill_target : natural range 0 to 1 := 0;
  signal fill_addr   : std_logic_vector(23 downto 0) := (others => '0');

  -- busy/done/line_valid on qspi_read_engine are LEVELS cleared only on
  -- the NEXT start -- a stale done='1' from a PRIOR fill would still be
  -- '1' for a cycle or two after we issue a new start (until the
  -- engine's own S_IDLE->S_CMD transition clears it). So completion is
  -- detected here as a busy-high-then-low edge (fill_seen_busy tracks
  -- "engine has genuinely gone busy since this fill's start"), never by
  -- sampling eng_done directly.
  signal fill_seen_busy : std_logic := '0';

  -- deferred demand-miss request (latched while its fill is outstanding)
  signal demand_pending : std_logic := '0';
  signal req_aligned    : std_logic_vector(23 downto 0) := (others => '0');
  signal req_widx       : natural range 0 to 7 := 0;
  signal demand_victim  : natural range 0 to 1 := 0;

  signal last_filled : natural range 0 to 1 := 1; -- alternator: first miss -> buf0

  -- internal registered ack (drives db_o.ack); the burst-mode read-ack strobe
  -- (port ack_r above) is a separate register, asserted only for READ beats
  -- (never for the "write, no effect" ack), per the sdram_ctrl contract.
  signal db_ack_r : std_logic := '0';
  signal rd_ack_r : std_logic := '0';
  signal d_r      : std_logic_vector(31 downto 0) := (others => '0');

  -- burst streaming state: once a burst request (bst='1') resolves (HIT or
  -- completed fill) to a full 32-byte line, the 8 words are streamed out
  -- back-to-back (one word/cycle, no wait states -- the line is already a
  -- register, unlike sdram_ctrl's per-beat DRAM access), asserting
  -- db_o.ack/ack_r each cycle, wcnt 0->7, matching the sdram_ctrl per-ack
  -- cadence the requester follows. bst_src records which buffer holds the
  -- streamed line, so the cnt=6/7 prefetch check (mirroring the HIT-path
  -- prefetch below) can target the OTHER buffer.
  signal bst_active : std_logic := '0';
  signal bst_words  : words_t := (others => (others => '0'));
  signal bst_cnt    : natural range 0 to 7 := 0;
  signal bst_tag    : std_logic_vector(23 downto 0) := (others => '0');
  signal bst_src    : natural range 0 to 1 := 0;
  signal bst_demand : std_logic := '0'; -- the outstanding demand_pending miss is a burst request

  function unpack_words(line : std_logic_vector(255 downto 0)) return words_t is
    variable w : words_t;
  begin
    for i in 0 to 7 loop
      w(i) := line(255 - 32*i downto 224 - 32*i);
    end loop;
    return w;
  end function;

begin

  eng : entity work.qspi_read_engine
    generic map (LANES => LANES, DUMMY_CYCLES => DUMMY_CYCLES)
    port map (
      clk        => clk,
      rst        => rst,
      start      => eng_start,
      start_addr => eng_start_addr,
      busy       => eng_busy,
      done       => eng_done,
      line_o     => eng_line_o,
      line_valid => eng_line_valid,
      cs_n       => fl_cs_n,
      sck        => fl_sck,
      io_o       => fl_io_o,
      io_oe      => fl_io_oe,
      io_i       => fl_io_i);

  process (clk) is
    variable flash_base_u   : unsigned(31 downto 0);
    variable a_u            : unsigned(31 downto 0);
    variable in_range       : boolean;
    variable addr_flash24   : unsigned(23 downto 0);
    variable aligned        : std_logic_vector(23 downto 0);
    variable widx           : natural range 0 to 7;
    variable hit0, hit1     : boolean;
    variable hit_words      : words_t;
    variable victim         : natural range 0 to 1;
    variable next_aligned   : std_logic_vector(23 downto 0);
    variable other_has_next : boolean;
    variable want_prefetch  : boolean;
    variable demand_resolved_now : boolean;
    variable bst_next_aligned   : std_logic_vector(23 downto 0);
    variable bst_other_has_next : boolean;
    variable bst_want_prefetch  : boolean;
    variable bst_victim         : natural range 0 to 1;
  begin
    if rising_edge(clk) then
      if rst = '1' then
        buf0           <= LBUF_RESET;
        buf1           <= LBUF_RESET;
        eng_start      <= '0';
        fill_kind      <= 0;
        fill_seen_busy <= '0';
        demand_pending <= '0';
        last_filled    <= 1;
        db_ack_r       <= '0';
        rd_ack_r       <= '0';
        d_r            <= (others => '0');
        bst_active     <= '0';
        bst_cnt        <= 0;
        bst_demand     <= '0';
      else
        eng_start <= '0'; -- default: pulse
        db_ack_r  <= '0'; -- default: single-cycle ack
        rd_ack_r  <= '0'; -- default: no read-ack strobe
        demand_resolved_now := false;

        if eng_busy = '1' then
          fill_seen_busy <= '1'; -- latch: engine has genuinely gone busy
        end if;

        flash_base_u := unsigned(FLASH_BASE);
        a_u          := unsigned(db_i.a);
        in_range     := (a_u >= flash_base_u) and (a_u < flash_base_u + FLASH_SIZE);

        ----------------------------------------------------------------
        -- burst streaming: once a burst request has resolved to a full
        -- line (HIT or completed fill, below), stream its 8 words
        -- back-to-back, one word/cycle, asserting db_o.ack/ack_r each
        -- cycle (wcnt 0->7) -- the sdram_ctrl per-ack cadence. This takes
        -- priority over accepting a new bus request.
        ----------------------------------------------------------------
        if bst_active = '1' then
          d_r      <= bst_words(bst_cnt);
          db_ack_r <= '1';
          rd_ack_r <= '1';

          -- mirror the HIT-path sequential run-ahead prefetch at the same
          -- word indices (6,7 of 0..7), targeting the buffer NOT holding
          -- the line being streamed.
          if bst_cnt >= PREFETCH_THRESHOLD and demand_pending = '0' then
            bst_next_aligned := std_logic_vector(unsigned(bst_tag) + 32);
            bst_victim       := 1 - bst_src;

            bst_other_has_next :=
              (bst_victim = 0 and buf0.valid = '1' and buf0.tag = bst_next_aligned) or
              (bst_victim = 1 and buf1.valid = '1' and buf1.tag = bst_next_aligned);

            bst_want_prefetch := not bst_other_has_next;

            if bst_want_prefetch and eng_busy = '0' and fill_kind = 0 then
              eng_start      <= '1';
              eng_start_addr <= bst_next_aligned;
              fill_kind      <= 2;
              fill_target    <= bst_victim;
              fill_addr      <= bst_next_aligned;
              fill_seen_busy <= '0';
            end if;
          end if;

          if bst_cnt = 7 then
            bst_active <= '0';
          else
            bst_cnt <= bst_cnt + 1;
          end if;

        ----------------------------------------------------------------
        -- bus side: new request (guarded on the PREVIOUS cycle's ack,
        -- read here as the old value of db_ack_r before this cycle's
        -- default/overrides take effect -- same pattern as gpio2.vhd)
        ----------------------------------------------------------------
        elsif db_i.en = '1' and db_ack_r = '0' then
          if db_i.wr = '1' and in_range then
            -- read-only region: ack, no effect (not a read beat: no rd_ack_r)
            db_ack_r <= '1';
          elsif db_i.rd = '1' and in_range then
            addr_flash24 := resize(a_u - flash_base_u, 24);
            aligned      := std_logic_vector(addr_flash24(23 downto 5)) & "00000";
            widx         := to_integer(addr_flash24(4 downto 2));

            hit0 := (buf0.valid = '1') and (buf0.tag = aligned);
            hit1 := (buf1.valid = '1') and (buf1.tag = aligned);

            if hit0 or hit1 then
              ------------------------------------------------------------
              -- HIT: serve immediately (single word, or kick off an
              -- 8-beat burst stream over the whole line for bst='1')
              ------------------------------------------------------------
              if hit0 then
                hit_words := buf0.words;
              else
                hit_words := buf1.words;
              end if;

              if bst = '1' then
                bst_words  <= hit_words;
                bst_tag    <= aligned;
                if hit0 then
                  bst_src <= 0;
                else
                  bst_src <= 1;
                end if;
                bst_active <= '1';
                bst_cnt    <= 1;
                d_r        <= hit_words(0);
              else
                d_r <= hit_words(widx);
              end if;
              db_ack_r <= '1';
              rd_ack_r <= '1';

              -- If this HIT happens to satisfy an already-pending demand
              -- miss (its line became valid via a background prefetch
              -- completing before the original miss's own fill/kick got
              -- to it), cancel that now-redundant pending demand so the
              -- deferred-kick logic below doesn't restart a needless
              -- fill and later fire a phantom/duplicate ack.
              if demand_pending = '1' and req_aligned = aligned then
                demand_pending      <= '0';
                demand_resolved_now := true; -- suppress the kick block below THIS cycle
              end if;

              -- sequential run-ahead prefetch (demand has priority: only
              -- when no demand fill is pending, and only into the buffer
              -- not just hit). For a burst request the same check is
              -- repeated during streaming (above) at cnt 6/7; skip it
              -- here to avoid a duplicate/premature fill kick.
              if bst = '0' and widx >= PREFETCH_THRESHOLD and demand_pending = '0' then
                next_aligned := std_logic_vector(unsigned(aligned) + 32);
                if hit0 then
                  victim := 1;
                else
                  victim := 0;
                end if;

                other_has_next :=
                  (victim = 0 and buf0.valid = '1' and buf0.tag = next_aligned) or
                  (victim = 1 and buf1.valid = '1' and buf1.tag = next_aligned);

                want_prefetch := not other_has_next;

                if want_prefetch and eng_busy = '0' and fill_kind = 0 then
                  eng_start      <= '1';
                  eng_start_addr <= next_aligned;
                  fill_kind      <= 2;
                  fill_target    <= victim;
                  fill_addr      <= next_aligned;
                  fill_seen_busy <= '0';
                end if;
              end if;

            else
              ------------------------------------------------------------
              -- MISS: latch request, start (or queue) a demand fill
              ------------------------------------------------------------
              if demand_pending = '0' then
                demand_pending <= '1';
                req_aligned    <= aligned;
                req_widx       <= widx;
                bst_demand     <= bst;
                victim         := 1 - last_filled;
                demand_victim  <= victim;

                if eng_busy = '0' and fill_kind = 0 then
                  eng_start      <= '1';
                  eng_start_addr <= aligned;
                  fill_kind      <= 1;
                  fill_target    <= victim;
                  fill_addr      <= aligned;
                  fill_seen_busy <= '0';
                end if;
                -- else: engine busy (a prefetch is in flight); the
                -- deferred-kick block below starts the demand fill as
                -- soon as the engine frees up.
              end if;
            end if;
          end if;
        end if;

        ----------------------------------------------------------------
        -- engine completion: commit the fill to its target buffer, and
        -- (for a demand fill) produce the deferred bus response --
        -- either a single word (bst_demand='0') or kick off the 8-beat
        -- burst stream (bst_demand='1') over the freshly-filled line.
        ----------------------------------------------------------------
        if fill_kind /= 0 and fill_seen_busy = '1' and eng_busy = '0' then
          if fill_target = 0 then
            buf0.tag   <= fill_addr;
            buf0.valid <= '1';
            buf0.words <= unpack_words(eng_line_o);
          else
            buf1.tag   <= fill_addr;
            buf1.valid <= '1';
            buf1.words <= unpack_words(eng_line_o);
          end if;
          last_filled <= fill_target;

          if fill_kind = 1 then -- demand fill: complete the deferred response
            if bst_demand = '1' then
              bst_words  <= unpack_words(eng_line_o);
              bst_tag    <= fill_addr;
              bst_src    <= fill_target;
              bst_active <= '1';
              bst_cnt    <= 1;
              d_r        <= unpack_words(eng_line_o)(0);
            else
              d_r <= unpack_words(eng_line_o)(req_widx);
            end if;
            db_ack_r       <= '1';
            rd_ack_r       <= '1';
            demand_pending <= '0';
            bst_demand     <= '0';
          end if;

          fill_kind <= 0;
        end if;

        ----------------------------------------------------------------
        -- kick a demand fill that was deferred because the engine was
        -- busy running a prefetch when the miss was detected
        ----------------------------------------------------------------
        if demand_pending = '1' and fill_kind = 0 and eng_busy = '0'
           and not demand_resolved_now then
          eng_start      <= '1';
          eng_start_addr <= req_aligned;
          fill_kind      <= 1;
          fill_target    <= demand_victim;
          fill_addr      <= req_aligned;
          fill_seen_busy <= '0';
        end if;

      end if;
    end if;
  end process;

  db_o.ack <= db_ack_r;
  db_o.d   <= d_r;
  ack_r    <= rd_ack_r;

end architecture;
