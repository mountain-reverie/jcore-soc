library ieee;
use ieee.std_logic_1164.all;
use ieee.numeric_std.all;
use work.cpu2j0_pack.all;

entity bootram_infer_tb is end entity;

architecture sim of bootram_infer_tb is
  signal clk : std_logic := '0';
  signal ibus_i : cpu_instruction_o_t := (en => '0', a => (others => '0'), jp => '0');
  signal ibus_o : cpu_instruction_i_t;
  signal db_i : cpu_data_o_t := (en => '0', a => (others => '0'), rd => '0', wr => '0', we => "0000", d => (others => '0'));
  signal db_o : cpu_data_i_t;
  signal done : boolean := false;
begin
  uut : entity work.bootram_infer(inferred)
    generic map (c_addr_width => 14)
    port map (clk => clk, ibus_i => ibus_i, ibus_o => ibus_o, db_i => db_i, db_o => db_o);

  -- 40 ns period clock
  clk <= not clk after 20 ns when not done else '0';

  stim : process
  begin
    -- Data read of word 0 (expect x"deadbeef" from the test boot_image_pkg)
    wait until rising_edge(clk);
    db_i.en <= '1'; db_i.a <= (others => '0'); db_i.rd <= '1';
    wait until rising_edge(clk);   -- falling edge in between latched the read
    assert db_o.ack = '1' report "data ack not asserted" severity failure;
    assert db_o.d = x"deadbeef" report "word0 readback wrong" severity failure;
    db_i.en <= '0'; db_i.rd <= '0';

    -- Instruction read of halfword at a=2 (word0 low half) expect x"beef"
    wait until rising_edge(clk);
    ibus_i.en <= '1'; ibus_i.a <= (1 => '1', others => '0');  -- a(1)='1' -> low half
    wait until rising_edge(clk);
    assert ibus_o.ack = '1' report "instr ack not asserted" severity failure;
    assert ibus_o.d = x"beef" report "instr low-half wrong" severity failure;
    ibus_i.a <= (others => '0');  -- a(1)='0' -> high half
    wait until rising_edge(clk);
    assert ibus_o.d = x"dead" report "instr high-half wrong" severity failure;
    ibus_i.en <= '0';

    -- Byte write: write x"aa" to byte lane 0 (bits 7:0) of word 0, then read back
    wait until rising_edge(clk);
    db_i.en <= '1'; db_i.a <= (others => '0'); db_i.wr <= '1'; db_i.we <= "0001";
    db_i.d <= x"000000aa";
    wait until rising_edge(clk);
    db_i.wr <= '0'; db_i.we <= "0000"; db_i.rd <= '1';
    wait until rising_edge(clk);
    assert db_o.d = x"deadbeaa" report "byte write-readback wrong" severity failure;
    db_i.en <= '0'; db_i.rd <= '0';

    -- Byte lanes 1,2,3 on word0 (currently x"deadbeaa") to cover we mapping:
    -- we(3)->31:24, we(2)->23:16, we(1)->15:8.
    wait until rising_edge(clk);
    db_i.en <= '1'; db_i.a <= (others => '0'); db_i.wr <= '1';
    db_i.we <= "1110"; db_i.d <= x"99887700";
    wait until rising_edge(clk);
    db_i.wr <= '0'; db_i.we <= "0000"; db_i.rd <= '1';
    wait until rising_edge(clk);
    assert db_o.d = x"998877aa" report "byte lanes 1-3 write wrong" severity failure;
    db_i.en <= '0'; db_i.rd <= '0';

    -- Non-zero word address: read word 1 (byte addr 4, a(2)='1') = x"12340000".
    wait until rising_edge(clk);
    db_i.en <= '1'; db_i.a <= (2 => '1', others => '0'); db_i.rd <= '1';
    wait until rising_edge(clk);
    assert db_o.d = x"12340000" report "word1 data readback wrong" severity failure;
    db_i.en <= '0'; db_i.rd <= '0';

    -- Instruction fetch at word 1 high half (a(2)='1', a(1)='0') = x"1234".
    wait until rising_edge(clk);
    ibus_i.en <= '1'; ibus_i.a <= (2 => '1', others => '0');
    wait until rising_edge(clk);
    assert ibus_o.d = x"1234" report "word1 instr high-half wrong" severity failure;
    ibus_i.en <= '0';

    report "bootram_infer_tb PASSED" severity note;
    done <= true;
    wait;
  end process;
end architecture;
