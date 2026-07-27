library ieee;
use ieee.std_logic_1164.all;
use ieee.numeric_std.all;
use work.cpu2j0_pack.all;
use work.boot_image_pkg.all;

entity bootram_infer is
  generic (
    c_addr_width : integer range 11 to 14 := 14;
    -- Default false preserves the original falling-edge/0-wait-state timing
    -- exactly (icesugar, the GF180 ASIC target and this file's own testbench
    -- all rely on that byte-identical behaviour). Only ULX3S instantiations
    -- set this true to move the capture to the rising edge for Fmax (see the
    -- process and ack comments below) at the cost of one wait state/access.
    RISING_EDGE_READ : boolean := false);
  port (
    clk    : in  std_logic;
    ibus_i : in  cpu_instruction_o_t;
    ibus_o : out cpu_instruction_i_t;
    db_i   : in  cpu_data_o_t;
    db_o   : out cpu_data_i_t);
end entity;

architecture inferred of bootram_infer is
  -- word-addressed: word index uses address bits (c_addr_width-1 downto 2)
  constant WORDS : integer := 2 ** (c_addr_width - 2);
  subtype word_t is std_logic_vector(31 downto 0);
  type mem_t is array (0 to WORDS - 1) of word_t;

  -- initialise from the generated boot image (zero-filled past the program)
  function init_mem return mem_t is
    variable m : mem_t := (others => (others => '0'));
  begin
    for i in 0 to WORDS - 1 loop
      if i < BOOT_DEPTH then
        m(i) := BOOT_IMAGE(i);
      end if;
    end loop;
    return m;
  end function;

  signal mem : mem_t := init_mem;

  signal d_word : word_t := (others => '0');
  signal i_word : word_t := (others => '0');
  signal i_half : std_logic := '0';

  -- Only used when RISING_EDGE_READ: 1-cycle-delayed ack that accompanies
  -- moving the capture from the falling edge (clkn-style) to the rising edge
  -- (clk). See the process and the db_o.ack/ibus_o.ack assignments below.
  signal db_ack_r   : std_logic := '0';
  signal ibus_ack_r : std_logic := '0';
begin
  -- synthesis translate_off
  assert BOOT_DEPTH <= WORDS
    report "boot_image_pkg BOOT_DEPTH exceeds boot RAM depth; image truncated"
    severity warning;
  -- synthesis translate_on
  -- Data port (read/write) and instruction port (read-only). RISING_EDGE_READ
  -- is a generic (elaboration-time constant), so exactly one of the two edge
  -- branches below is ever active in a given instance; this is not a runtime
  -- mux.
  --
  -- Default (RISING_EDGE_READ = false): capture on the FALLING edge so
  -- registered output is valid the same cycle ack=en is asserted, matching
  -- memory_fpga's 0-wait contract (bus delays are FALSE). This is the
  -- half-cycle path: the CPU asserts en+address at a rising edge, the
  -- intervening falling edge clocks the data, and the CPU samples it at the
  -- NEXT rising edge. Kept as the default so icesugar, the GF180 ASIC target
  -- and this file's own testbench remain byte-identical.
  --
  -- RISING_EDGE_READ = true (ULX3S only): capture on the RISING edge instead,
  -- giving this path a full clock period instead of half -- this is what
  -- removes bootram_infer as the ECP5 Fmax limiter. Read data now lands one
  -- cycle later than before, so ack is delayed a cycle to match (below); every
  -- access costs one additional wait state.
  process(clk)
    variable di : integer;
    variable capture_edge : boolean;
  begin
    if RISING_EDGE_READ then
      capture_edge := rising_edge(clk);
    else
      capture_edge := falling_edge(clk);
    end if;
    if capture_edge then
      -- data port
      di := to_integer(unsigned(db_i.a(c_addr_width - 1 downto 2)));
      if db_i.en = '1' and db_i.wr = '1' then
        if db_i.we(0) = '1' then mem(di)(7 downto 0)   <= db_i.d(7 downto 0);   end if;
        if db_i.we(1) = '1' then mem(di)(15 downto 8)  <= db_i.d(15 downto 8);  end if;
        if db_i.we(2) = '1' then mem(di)(23 downto 16) <= db_i.d(23 downto 16); end if;
        if db_i.we(3) = '1' then mem(di)(31 downto 24) <= db_i.d(31 downto 24); end if;
      end if;
      -- READ_FIRST: d_word captures the pre-write value (matches memory_fpga).
      -- The read is unconditional (db_i.rd is ignored, as in memory_fpga); when
      -- the bus is idle d_word simply holds a stale value, which is harmless
      -- because ack is low then and the CPU never samples it.
      d_word <= mem(di);
      -- instruction port
      i_word <= mem(to_integer(unsigned(ibus_i.a(c_addr_width - 1 downto 2))));
      i_half <= ibus_i.a(1);
      -- ack for RISING_EDGE_READ: a one-cycle pulse per access that self-clears
      -- while en is still held, so back-to-back accesses (e.g. continuously
      -- asserted instruction fetch) each ack exactly once. A plain registered
      -- `en` is WRONG here: for a continuously-enabled port it would go high
      -- on the second cycle and then STAY high while the data lags one cycle
      -- behind, so the CPU would consume the wrong (stale) word every single
      -- cycle -- a hang, not a slowdown. Unused (left low) when
      -- RISING_EDGE_READ is false.
      if RISING_EDGE_READ then
        db_ack_r   <= db_i.en   and not db_ack_r;
        ibus_ack_r <= ibus_i.en and not ibus_ack_r;
      end if;
    end if;
  end process;

  -- Timing contract: default (RISING_EDGE_READ=false, matches memory_fpga,
  -- boot-mem bus delays FALSE) ack is combinational = en, valid the same
  -- cycle since d_word/i_word were captured on the falling edge in between.
  -- RISING_EDGE_READ=true: ack is the registered, 1-cycle-delayed version
  -- above, since d_word/i_word are now only valid one cycle after en.
  db_o.d   <= d_word;
  db_o.ack <= db_ack_r when RISING_EDGE_READ else db_i.en;

  -- big-endian halfword select: a(1)='0' -> high half (bits 31:16)
  ibus_o.d   <= i_word(31 downto 16) when i_half = '0' else i_word(15 downto 0);
  ibus_o.ack <= ibus_ack_r when RISING_EDGE_READ else ibus_i.en;
end architecture;
