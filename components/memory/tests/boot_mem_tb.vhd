-- Unit testbench for bootram_infer(boot_mem): the silicon-correct boot
-- memory (boot-memory refinement -- scratchpad removal) --
--   0x000-0xFFF : read-only constant ROM (SH-2 reset vector, from
--                 boot_image_pkg.BOOT_IMAGE), synthesizes to gates. There
--                 is no writable stack lane any more -- SDRAM self-inits
--                 in hardware, so the reset SP points into SDRAM instead
--                 (see targets/asic/gf180_j4mmu/boot_image_pkg.vhd).
--
-- Exercises (severity failure on any mismatch):
--   (a) db read of word0 (a=0x0) = BOOT_IMAGE(0) = PC = 0x14000000, and
--       word1 (a=0x4) = BOOT_IMAGE(1) = SP (now an SDRAM address).
--   (b) a db WRITE anywhere (a=0x0, a=0xFFC) is ignored -- a later read
--       still returns the ROM constant.
--   (c) an ibus read of a=0x0 returns the same ROM word (high half, per
--       the (inferred) architecture's half-select convention: a(1)='0' ->
--       high half of the word).
library ieee;
use ieee.std_logic_1164.all;
use ieee.numeric_std.all;
use work.cpu2j0_pack.all;
use work.boot_image_pkg.all;

entity boot_mem_tb is end entity;

architecture sim of boot_mem_tb is
  constant C_ADDR_WIDTH : integer := 12;

  signal clk  : std_logic := '0';
  signal done : boolean := false;

  signal ibus_i : cpu_instruction_o_t := (en => '0', a => (others => '0'), jp => '0');
  signal ibus_o : cpu_instruction_i_t;
  signal db_i   : cpu_data_o_t := (en => '0', a => (others => '0'), rd => '0', wr => '0', we => "0000", d => (others => '0'));
  signal db_o   : cpu_data_i_t;
begin
  uut : entity work.bootram_infer(boot_mem)
    generic map (c_addr_width => C_ADDR_WIDTH)
    port map (clk => clk, ibus_i => ibus_i, ibus_o => ibus_o, db_i => db_i, db_o => db_o);

  clk <= not clk after 20 ns when not done else '0';

  stim : process
    procedure db_write(addr : integer; word_val : std_logic_vector(31 downto 0);
                        we : std_logic_vector(3 downto 0) := "1111") is
    begin
      wait until rising_edge(clk);
      db_i.en <= '1'; db_i.a <= std_logic_vector(to_unsigned(addr, 32));
      db_i.wr <= '1'; db_i.we <= we; db_i.d <= word_val;
      wait until rising_edge(clk);
      db_i.en <= '0'; db_i.wr <= '0'; db_i.we <= "0000";
    end procedure;

    -- reads via the data port; data is registered on the falling edge that
    -- follows en/a being asserted at a rising edge, and sampled at the NEXT
    -- rising edge (mirrors (inferred)'s 0-wait falling-edge contract).
    procedure db_read(addr : integer; expect : std_logic_vector(31 downto 0); msg : string) is
    begin
      wait until rising_edge(clk);
      db_i.en <= '1'; db_i.a <= std_logic_vector(to_unsigned(addr, 32)); db_i.rd <= '1';
      wait until rising_edge(clk);
      assert db_o.ack = '1' report "boot_mem: db ack not asserted (" & msg & ")" severity failure;
      assert db_o.d = expect report "boot_mem: db data mismatch (" & msg & ")" severity failure;
      db_i.en <= '0'; db_i.rd <= '0';
    end procedure;

    procedure ibus_read(addr : integer; expect_half : std_logic_vector(15 downto 0);
                         half : std_logic; msg : string) is
      variable a_bits : std_logic_vector(31 downto 0);
    begin
      a_bits := std_logic_vector(to_unsigned(addr, 32));
      wait until rising_edge(clk);
      ibus_i.en <= '1'; ibus_i.a <= a_bits(31 downto 2) & half;
      wait until rising_edge(clk);
      assert ibus_o.ack = '1' report "boot_mem: ibus ack not asserted (" & msg & ")" severity failure;
      assert ibus_o.d = expect_half report "boot_mem: ibus data mismatch (" & msg & ")" severity failure;
      ibus_i.en <= '0';
    end procedure;
  begin
    wait until rising_edge(clk);
    wait until rising_edge(clk);

    -- (a) ROM reads return the boot-image constants.
    db_read(16#000#, BOOT_IMAGE(0), "rom word0 = PC");
    db_read(16#004#, BOOT_IMAGE(1), "rom word1 = SP");

    -- (b) writes anywhere in the window are ignored (no write port).
    db_write(16#000#, x"deadbeef");
    db_read(16#000#, BOOT_IMAGE(0), "rom write ignored (word0)");

    db_write(16#ffc#, x"12345678");
    db_read(16#ffc#, x"00000000", "rom write ignored (word 0xffc, past BOOT_DEPTH)");

    -- (c) ibus read of the ROM word (high half, a(1)='0' per (inferred)).
    ibus_read(16#000#, BOOT_IMAGE(0)(31 downto 16), '0', "ibus rom high half");
    ibus_read(16#000#, BOOT_IMAGE(0)(15 downto 0),  '1', "ibus rom low half");

    report "boot_mem OK" severity note;
    done <= true;
    wait;
  end process;
end architecture;
