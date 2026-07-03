library ieee; use ieee.std_logic_1164.all; use ieee.numeric_std.all;
use work.cpu2j0_pack.all;

entity dev_ddr_spram_tb is end entity;
architecture sim of dev_ddr_spram_tb is
  signal clk    : std_logic := '0';
  signal ibus_i : cpu_instruction_o_t := NULL_INST_O;
  signal ibus_o : cpu_instruction_i_t;
  signal dbus_i : cpu_data_o_t := NULL_DATA_O;
  signal dbus_o : cpu_data_i_t;
  signal done   : boolean := false;

  -- word X: word index 5 -> byte address 20 (0b1_0100); a(16:2)=5, a(1)=0 selects
  -- the upper 16-bit half, a(1)=1 (byte address 22) selects the lower half.
  -- Build the instruction address as a true bit-for-bit slice (a(31:1)) of the
  -- equivalent 32-bit byte address, so a(k) always means "address bit k" the
  -- same way for both buses (to_unsigned(_,31) would misalign by one position
  -- against a 32-bit value since the target range starts at index 1, not 0).
  constant WORD_X_DATA_A     : std_logic_vector(31 downto 0) := std_logic_vector(to_unsigned(20, 32));
  constant WORD_X_LO_BYTE_A  : std_logic_vector(31 downto 0) := std_logic_vector(to_unsigned(22, 32));
  constant WORD_X_INSTR_A_HI : std_logic_vector(31 downto 1) := WORD_X_DATA_A(31 downto 1);
  constant WORD_X_INSTR_A_LO : std_logic_vector(31 downto 1) := WORD_X_LO_BYTE_A(31 downto 1);
begin
  uut : entity work.dev_ddr_spram
    port map (clk => clk, ibus_i => ibus_i, ibus_o => ibus_o, dbus_i => dbus_i, dbus_o => dbus_o);

  clk <= not clk after 5 ns when not done else '0';

  -- The adapter drives sp_en/sp_a combinationally from the bus inputs, and
  -- spram_128k returns dr with exactly 1-cycle registered latency (matching
  -- the adapter's own r_data_ack/r_instr_ack registers). So a request
  -- presented during interval I is sampled at the next rising edge and its
  -- ack/data are visible starting the interval right after that single tick.
  stim : process
    procedure tick is begin wait until rising_edge(clk); wait for 1 ns; end procedure;
  begin
    -- 1. data write of word X
    dbus_i.en <= '1'; dbus_i.we <= "1111"; dbus_i.a <= WORD_X_DATA_A; dbus_i.d <= x"A5A51234";
    tick; -- write commits this edge
    dbus_i.we <= "0000"; dbus_i.d <= (others => '0'); dbus_i.en <= '0';
    tick; -- idle: write ack (unchecked) clears

    -- 1b. data read of the same word -> ack + data at N+1
    dbus_i.en <= '1'; dbus_i.a <= WORD_X_DATA_A;
    tick;
    assert dbus_o.ack = '1' report "data read ack missing" severity failure;
    assert dbus_o.d = x"A5A51234" report "data read value mismatch" severity failure;
    dbus_i.en <= '0';
    tick; -- idle
    assert dbus_o.ack = '0' report "data ack should deassert when idle" severity failure;

    -- 2. instruction fetch of the same word, upper half (a(1)=0)
    ibus_i.en <= '1'; ibus_i.a <= WORD_X_INSTR_A_HI;
    tick;
    assert ibus_o.ack = '1' report "instr fetch ack missing (hi)" severity failure;
    assert ibus_o.d = x"A5A5" report "instr upper-half mismatch" severity failure;
    ibus_i.en <= '0';
    tick; -- idle

    -- lower half (a(1)=1)
    ibus_i.en <= '1'; ibus_i.a <= WORD_X_INSTR_A_LO;
    tick;
    assert ibus_o.ack = '1' report "instr fetch ack missing (lo)" severity failure;
    assert ibus_o.d = x"1234" report "instr lower-half mismatch" severity failure;
    ibus_i.en <= '0';
    tick; -- idle

    -- 3. simultaneous ibus_i.en and dbus_i.en: data wins, instr stalls
    dbus_i.en <= '1'; dbus_i.we <= "0000"; dbus_i.a <= WORD_X_DATA_A;
    ibus_i.en <= '1'; ibus_i.a <= WORD_X_INSTR_A_HI;
    tick; -- both requests presented; data-priority arbitration resolves this edge
    assert dbus_o.ack = '1' report "data should win simultaneous arbitration" severity failure;
    assert dbus_o.d = x"A5A51234" report "data value wrong after arbitration win" severity failure;
    assert ibus_o.ack = '0' report "instr should be stalled during simultaneous access" severity failure;

    -- data access done; instr keeps re-issuing (pipeline holds ibus_i.en/a)
    -- and completes alone next cycle since dbus_i.en is now low
    dbus_i.en <= '0';
    tick;
    assert ibus_o.ack = '1' report "stalled instr fetch did not complete next cycle" severity failure;
    assert ibus_o.d = x"A5A5" report "stalled instr fetch value mismatch" severity failure;

    ibus_i.en <= '0';
    tick;

    report "dev_ddr_spram_tb PASSED" severity note;
    done <= true; wait;
  end process;
end architecture;
