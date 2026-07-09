library ieee;
use ieee.std_logic_1164.all;
use ieee.numeric_std.all;
use work.cpu2j0_pack.all;
use work.boot_image_coremark_pkg.all;

-- bootram_infer_coremark: a copy of components/memory/bootram_infer.vhd that
-- imports work.boot_image_coremark_pkg instead of work.boot_image_pkg.
-- bootram_infer's use clause is hard-wired to the boot_image_pkg package
-- name (VHDL use clauses cannot be parameterized by a generic), so the
-- cleanest way to give cpus_coremark a different boot image without
-- disturbing the banner board's bootram_infer/boot_image_pkg pairing is this
-- small variant entity/architecture. Logic is otherwise identical to
-- bootram_infer(inferred).
entity bootram_infer_coremark is
  generic (c_addr_width : integer range 11 to 14 := 14);
  port (
    clk    : in  std_logic;
    ibus_i : in  cpu_instruction_o_t;
    ibus_o : out cpu_instruction_i_t;
    db_i   : in  cpu_data_o_t;
    db_o   : out cpu_data_i_t);
end entity;

architecture inferred of bootram_infer_coremark is
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
begin
  -- synthesis translate_off
  assert BOOT_DEPTH <= WORDS
    report "boot_image_coremark_pkg BOOT_DEPTH exceeds boot RAM depth; image truncated"
    severity warning;
  -- synthesis translate_on
  -- Data port (read/write) and instruction port (read-only), both on falling
  -- edge so registered output is valid the same cycle ack=en is asserted,
  -- matching memory_fpga's 0-wait contract (bus delays are FALSE).
  process(clk)
    variable di : integer;
  begin
    if falling_edge(clk) then
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
    end if;
  end process;

  -- Timing contract (matches memory_fpga, boot-mem bus delays are FALSE): ack
  -- is combinational = en, while d_word/i_word are registered from the falling
  -- edge. The CPU asserts en+address at a rising edge, the intervening falling
  -- edge clocks the data, and the CPU samples it at the NEXT rising edge (when
  -- ack, asserted since the request, is still seen). Data is NOT valid on the
  -- same rising edge en first rises.
  db_o.d   <= d_word;
  db_o.ack <= db_i.en;

  -- big-endian halfword select: a(1)='0' -> high half (bits 31:16)
  ibus_o.d   <= i_word(31 downto 16) when i_half = '0' else i_word(15 downto 0);
  ibus_o.ack <= ibus_i.en;
end architecture;
