library ieee;
use ieee.std_logic_1164.all;
use ieee.numeric_std.all;

-- Behavioral GHDL sim model of a DS3231 RTC as an I2C (bit-banged) slave at
-- 7-bit address 0x68. Sim-only: deterministic, no now()/random. Samples SDA
-- on the rising edge of SCL (driven by the bit-bang master in banner.c);
-- ACKs by pulling SDA low on the 9th clock. SDA is only ever driven '0'
-- (open-drain) or released to 'Z' -- the external pull-up (modeled in
-- components/emac/sb_io_sim.vhd via the SB_IO PULLUP generic) resolves the
-- released level high.
--
-- Register file 0x00..0x0F, only 0x00..0x06 (sec/min/hour/day/date/month/
-- year, all BCD) are meaningful for this test; 0x0E/0x0F (control/status)
-- are just storage. Register pointer auto-increments (wrapping mod 16) on
-- both read and write, matching the DS3231 datasheet.
--
-- NOT modeled: clock ticking (registers are static until written), alarms,
-- temperature. Out of scope per the driver spec. SQW is idle-high and, in
-- this sim model only, pulses a handful of times shortly after reset (see
-- sqw_pulse below) so a testbench can exercise the AIC irq_i(1) path; real
-- hardware SQW runs at a much slower, configurable rate.
entity ds3231_model is
  port (
    scl       : in    std_logic;
    sda       : inout std_logic;
    sqw       : out   std_logic := '1';
    reg_sec   : out   std_logic_vector(7 downto 0);
    reg_min   : out   std_logic_vector(7 downto 0);
    reg_hour  : out   std_logic_vector(7 downto 0);
    reg_day   : out   std_logic_vector(7 downto 0);
    reg_date  : out   std_logic_vector(7 downto 0);
    reg_month : out   std_logic_vector(7 downto 0);
    reg_year  : out   std_logic_vector(7 downto 0));
end entity;

architecture sim of ds3231_model is
  constant I2C_ADDR : std_logic_vector(6 downto 0) := "1101000";  -- 0x68

  type regfile_t is array (0 to 15) of std_logic_vector(7 downto 0);
  signal regs : regfile_t := (others => (others => '0'));

  -- Toggled once per START (incl. repeated-START) / STOP condition seen on
  -- the bus. The main FSM process waits on these (via 'event) rather than
  -- racing SDA/SCL edges directly, which keeps the byte-receive loop simple.
  signal start_tog : std_logic := '0';
  signal stop_tog  : std_logic := '0';
begin
  -- Sim-only fast SQW pulse generator: a real DS3231 SQW is a slow (typ.
  -- 1 Hz) square wave, far too slow to exercise the AIC interrupt path
  -- (irq_i(1), aic.vhd's rising-edge aic_edgedet) within a sim run of
  -- reasonable wall-clock length. Idle high (matching the datasheet/open-
  -- drain idle level and this entity's default), then pulse low/high
  -- continuously at a 1 ms period for the whole run.
  --
  -- Why free-run for the whole sim rather than a short early burst: the AIC
  -- edge-detect latch (components/misc/aic_edgedet.vhd) captures a rising
  -- edge ONLY while its enable es_irqs(1) is high -- "q <= en_i" on the
  -- edge -- and es_irqs(1) is 0 until the CPU writes aic0's ilevel(1)
  -- (banner.c's AIC0_ILEVELS write in main(), which only runs after
  -- ds3231_init()'s ~13 ms bit-banged I2C round trip). A burst that ends
  -- before that write would have every edge discarded. Free-running edges
  -- guarantee several rising edges land after the enable and before the
  -- AIC-check print (~148 ms, just past the SPRAM memtest). No now()/
  -- random: a fixed toggle loop, deterministic across runs.
  sqw_pulse : process
    constant SQW_HALF_PERIOD : time := 10 us;
  begin
    sqw <= '1';
    loop
      wait for SQW_HALF_PERIOD;
      sqw <= '0';
      wait for SQW_HALF_PERIOD;
      sqw <= '1';
    end loop;
  end process;

  reg_sec   <= regs(0);
  reg_min   <= regs(1);
  reg_hour  <= regs(2);
  reg_day   <= regs(3);
  reg_date  <= regs(4);
  reg_month <= regs(5);
  reg_year  <= regs(6);

  -- START/STOP detector: purely combinational on SDA edges while SCL='1'.
  monitor : process(sda)
  begin
    if to_X01(scl) = '1' then
      if falling_edge(sda) then
        start_tog <= not start_tog;
      elsif rising_edge(sda) then
        stop_tog <= not stop_tog;
      end if;
    end if;
  end process;

  fsm : process
    variable ptr       : integer range 0 to 15 := 0;
    variable byte      : std_logic_vector(7 downto 0);
    variable rw        : std_logic;
    variable matched   : boolean;
    variable master_ack : boolean;
    variable need_wait_start : boolean := true;
    variable have_event : boolean;

    -- Receive one byte MSB-first, sampling SDA on SCL rising edges. Does
    -- NOT drive the ACK bit -- caller decides after inspecting the byte.
    procedure recv_bits(b : out std_logic_vector(7 downto 0)) is
    begin
      b := (others => '0');
      for i in 7 downto 0 loop
        wait until rising_edge(scl);
        b(i) := to_X01(sda);
        wait until falling_edge(scl);
      end loop;
    end procedure;

    -- Drive the 9th (ACK) clock: pull SDA low for `ack`=true, else release.
    procedure drive_ack(ack : in boolean) is
    begin
      if ack then
        sda <= '0';
      else
        sda <= 'Z';
      end if;
      wait until falling_edge(scl);
      sda <= 'Z';
    end procedure;

    -- Send one byte MSB-first, data set up on the falling edge of SCL (so
    -- it is stable through the master's rising-edge sample), then release
    -- and sample the master's ACK/NACK on the 9th clock.
    procedure send_byte(b : in std_logic_vector(7 downto 0);
                         master_ack : out boolean) is
    begin
      for i in 7 downto 0 loop
        sda <= b(i);
        wait until rising_edge(scl);
        wait until falling_edge(scl);
      end loop;
      sda <= 'Z';
      wait until rising_edge(scl);
      master_ack := (to_X01(sda) = '0');
      wait until falling_edge(scl);
      sda <= 'Z';
    end procedure;
  begin
    sda <= 'Z';

    main_loop : loop
      -- Wait for a START condition, unless we already consumed one (a
      -- repeated-START) while breaking out of the read/write byte loops
      -- below -- in that case skip straight to the address byte.
      if need_wait_start then
        wait until start_tog'event;
      end if;
      need_wait_start := true;

      -- Address + R/W byte.
      recv_bits(byte);
      matched := (byte(7 downto 1) = I2C_ADDR);
      rw := byte(0);
      drive_ack(matched);

      exit main_loop when not matched;

      have_event := false;

      if rw = '0' then
        -- WRITE: first data byte is the register pointer; subsequent
        -- bytes auto-increment and store, until STOP/repeated-START.
        recv_bits(byte);
        ptr := to_integer(unsigned(byte(3 downto 0)));
        drive_ack(true);
        write_loop : loop
          -- Between bytes, either a fresh bit clock starts (more data) or
          -- a STOP/repeated-START condition begins. Note a repeated-START's
          -- own SCL-high phase (the master releasing SCL before pulling SDA
          -- low) *also* produces a rising_edge(scl) here, indistinguishable
          -- at that instant from "next data bit's clock". So every
          -- subsequent edge-wait in this byte-capture must keep racing
          -- start_tog/stop_tog too, and bail (discarding the partial byte)
          -- the moment either fires -- rather than committing to "this was
          -- data" as soon as the first rising edge is seen.
          wait until rising_edge(scl) or start_tog'event or stop_tog'event;
          if stop_tog'event or start_tog'event then
            have_event := true;
            exit write_loop;
          end if;
          byte := (others => '0');
          byte(7) := to_X01(sda);
          wait until falling_edge(scl) or start_tog'event or stop_tog'event;
          if stop_tog'event or start_tog'event then
            have_event := true;
            exit write_loop;
          end if;
          for i in 6 downto 0 loop
            wait until rising_edge(scl) or start_tog'event or stop_tog'event;
            if stop_tog'event or start_tog'event then
              have_event := true;
              exit write_loop;
            end if;
            byte(i) := to_X01(sda);
            wait until falling_edge(scl) or start_tog'event or stop_tog'event;
            if stop_tog'event or start_tog'event then
              have_event := true;
              exit write_loop;
            end if;
          end loop;
          regs(ptr) <= byte;
          ptr := (ptr + 1) mod 16;
          drive_ack(true);
        end loop;
      else
        -- READ: stream regs(ptr) MSB-first; stop on master NACK or STOP.
        -- send_byte's own bit loop simply waits for the master's next SCL
        -- edges, which works whether that is another read byte or (if the
        -- master instead issues STOP/repeated-START) never arrives -- so
        -- we only reach the STOP/START wait below after a master NACK.
        read_loop : loop
          send_byte(regs(ptr), master_ack);
          exit read_loop when not master_ack;
          ptr := (ptr + 1) mod 16;
        end loop;
      end if;

      if not have_event then
        wait until start_tog'event or stop_tog'event;
      end if;
      exit main_loop when stop_tog'event;
      -- else repeated-START: the start_tog event has already been
      -- consumed, so skip waiting for a fresh one at the top of the loop.
      need_wait_start := false;
    end loop;

    sda <= 'Z';
  end process;
end architecture;
