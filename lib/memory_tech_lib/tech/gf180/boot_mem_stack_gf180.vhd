-- tech/gf180 backend for bootram_infer(boot_mem) (see components/memory/
-- boot_mem.vhd): the SILICON-CORRECT boot memory used by the FLASH/XIP
-- variant of gf180_j4mmu (bound via bootram_infer(boot_mem), c_addr_width=>12,
-- see targets/asic/gf180_j4mmu/cpus_one_m0_gf180_arch.vhd). This architecture
-- reproduces boot_mem's split exactly:
--   0x000-0x7FF (is_rom='1'): the SAME constant-ROM lane as (boot_mem) --
--     kept as a plain VHDL constant array with NO write port, so it
--     synthesizes to LOGIC (gates), not a memory macro. Untouched here.
--   0x800-0xFFF (is_rom='0'): the 2 KB writable stack, now bound to 4x
--     vendor gf180mcu_fd_ip_sram__sram512x8m8wm1 hard-IP macros (one per
--     byte lane) instead of (boot_mem)'s inferred array-of-flops. This is
--     the REAL silicon area for the stack lane.
--
-- WHY NO DEPTH TILING (unlike the old unified 16KB bootram (removed)
-- 16 KiB bootram's gf180 backend): boot_mem's two lanes are each exactly
-- 2 KB = 512 words -- the vendor sram512x8m8wm1 macro's native depth. One
-- macro instance per byte lane covers the whole stack with no row/tile mux
-- needed at all (unlike the old 4096-word design, which needed 8 depth
-- tiles). This also means the ROM and SRAM lanes use the SAME 9-bit word
-- index range (a(c_addr_width-2 downto 2), c_addr_width=12 -> a(10 downto 2)),
-- selected between by a(c_addr_width-1) exactly as in (boot_mem).
--
-- ARBITRATION: the stack SRAM macro is single-port, but bootram_infer
-- exposes two logical ports (db: read/write, ibus: read-only). Both ports
-- read the ROM lane independently (cheap logic, no conflict). For the SRAM
-- lane, only ONE physical access can be served per cycle -- the data port
-- wins whenever it targets the SRAM lane (db_i.en='1' and a(11)='1'); the
-- instruction port is only served from the macro when the data port is
-- idle or targeting the ROM lane instead. This mirrors
-- the removed old-bootram ENABLING PRECONDITION: jcore's CPU boot sequence
-- never needs to fetch instructions FROM the writable stack window (code
-- lives in the ROM lane / external flash), so this is a synth/metrics-only
-- simplification with no functional-path impact (functional sim / FPGA
-- keep instantiating bootram_infer(boot_mem) directly, tech/inferred).
--
-- REGISTERED READ-DATA / EDGE ALIGNMENT: (boot_mem) registers d_word/i_word
-- off falling_edge(clk). The vendor macro's Q is a standard rising-edge
-- synchronous read (1-cycle latency). Clocking every macro from `not clk`
-- (its rising edge coincides with the system clock's falling edge) gives
-- byte-identical timing to (boot_mem) with zero extra pipeline stages --
-- same tiling technique as ram_2x8x2048_2rw_gf180.vhd.
--
-- SYNTH/METRICS-ONLY: this architecture instantiates a black-box vendor
-- macro (see gf180mcu_fd_ip_sram_comp.vhd) that GHDL cannot elaborate to a
-- functional model. It is NOT wired into the GHDL functional-sim path;
-- functional simulation / FPGA builds keep using bootram_infer(boot_mem)
-- (tech/inferred-equivalent, the default binding in
-- cpus_one_m0_gf180_arch.vhd / cpus_xip_probe.vhd). Selected only by the
-- gf180 LibreLane harden flow (targets/asic/gf180_j4mmu/librelane/
-- boot_mem/, run.sh macro=boot_mem).
library ieee;
use ieee.std_logic_1164.all;
use ieee.numeric_std.all;
use work.cpu2j0_pack.all;
use work.boot_image_pkg.all;
use work.gf180_sram_comp_pkg.all;

architecture boot_mem_gf180 of bootram_infer is
  constant ROM_WORDS  : integer := 2 ** (c_addr_width - 3); -- low half, word-addressed
  constant SRAM_WORDS : integer := 2 ** (c_addr_width - 3); -- high half, word-addressed
  subtype word_t is std_logic_vector(31 downto 0);
  type rom_t is array (0 to ROM_WORDS - 1) of word_t;

  -- read-only constant ROM lane: identical to (boot_mem)'s init_rom.
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

  signal d_rom_word : word_t := (others => '0');
  signal i_rom_word  : word_t := (others => '0');
  signal i_half      : std_logic := '0';

  signal clk_n : std_logic;

  -- Arbitration: data port has priority whenever it targets the SRAM lane
  -- (see header). Instruction port is only physically served from the
  -- macro when it targets the SRAM lane AND the data port is not also
  -- targeting the SRAM lane this cycle.
  signal db_sram_req   : std_logic;
  signal ibus_sram_req : std_logic;
  signal serving_db    : std_logic;
  signal active_a      : std_logic_vector(8 downto 0);
  signal op_write      : std_logic;

  signal serving_db_reg : std_logic := '0';
  signal d_word_reg     : std_logic := '0'; -- registered "was ROM or SRAM" select for db port
  signal i_word_reg     : std_logic := '0'; -- registered "was ROM or SRAM" select for ibus port

  signal i_sram_half_word : std_logic_vector(15 downto 0);
  signal i_rom_half_word  : std_logic_vector(15 downto 0);
  signal ibus_use_sram    : std_logic;

  constant WEN_ALL_ON : std_logic_vector(7 downto 0) := (others => '0');
  type lane_data_t is array (0 to 3) of std_logic_vector(7 downto 0);
  signal q_lanes : lane_data_t;
  signal q_word  : std_logic_vector(31 downto 0);
begin
  -- synthesis translate_off
  assert BOOT_DEPTH <= ROM_WORDS
    report "boot_image_pkg BOOT_DEPTH exceeds boot_mem ROM lane depth; image truncated"
    severity warning;
  -- synthesis translate_on

  clk_n <= not clk;

  db_sram_req   <= db_i.en   and db_i.a(c_addr_width - 1);
  ibus_sram_req <= ibus_i.en and ibus_i.a(c_addr_width - 1);
  serving_db    <= db_sram_req;

  active_a <= db_i.a(c_addr_width - 2 downto 2)   when serving_db = '1' else
              ibus_i.a(c_addr_width - 2 downto 2);
  op_write <= db_i.en and db_i.wr and db_sram_req;

  -- ROM lane: pure combinational-read constant array, one independent
  -- decoder per port -- synthesizes to gates, no macro, no arbitration.
  rom_reg : process(clk)
    variable ddi : integer;
    variable idi : integer;
  begin
    if falling_edge(clk) then
      ddi := to_integer(unsigned(db_i.a(c_addr_width - 2 downto 2)));
      d_rom_word <= rom(ddi);
      d_word_reg <= db_i.a(c_addr_width - 1);

      idi := to_integer(unsigned(ibus_i.a(c_addr_width - 2 downto 2)));
      i_rom_word <= rom(idi);
      i_half     <= ibus_i.a(1);
      i_word_reg <= ibus_i.a(c_addr_width - 1);
    end if;
  end process;

  reg_sel : process(clk_n)
  begin
    if rising_edge(clk_n) then
      serving_db_reg <= serving_db;
    end if;
  end process;

  lane_gen : for l in 0 to 3 generate
    signal cen  : std_logic;
    signal gwen : std_logic;
  begin
    cen  <= not (db_sram_req or ibus_sram_req);
    gwen <= not (op_write and db_i.we(l));

    sram_i : \gf180mcu_fd_ip_sram__sram512x8m8wm1\
      port map (
        CLK  => clk_n,
        CEN  => cen,
        GWEN => gwen,
        WEN  => WEN_ALL_ON,
        A    => active_a,
        D    => db_i.d((l + 1) * 8 - 1 downto l * 8),
        Q    => q_lanes(l));
  end generate;

  q_word <= q_lanes(3) & q_lanes(2) & q_lanes(1) & q_lanes(0);

  -- Data port mux: SRAM word if the registered access targeted the SRAM
  -- lane (and was actually the one served -- data port always wins when it
  -- requests SRAM, so d_word_reg='1' implies serving_db_reg='1'), else ROM.
  db_o.d   <= q_word when d_word_reg = '1' else d_rom_word;
  db_o.ack <= db_i.en;

  -- Instruction port mux: SRAM word only if it targeted the SRAM lane AND
  -- was the one actually served that cycle (data port could have won
  -- arbitration instead); otherwise ROM (matches (boot_mem)'s convention of
  -- returning the ROM/SRAM word regardless of contention -- see header).
  i_sram_half_word <= q_word(31 downto 16) when i_half = '0' else q_word(15 downto 0);
  i_rom_half_word  <= i_rom_word(31 downto 16) when i_half = '0' else i_rom_word(15 downto 0);
  ibus_use_sram    <= i_word_reg and not serving_db_reg;

  ibus_o.d   <= i_sram_half_word when ibus_use_sram = '1' else i_rom_half_word;
  ibus_o.ack <= ibus_i.en;
end architecture;
