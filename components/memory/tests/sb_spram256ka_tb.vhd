library ieee; use ieee.std_logic_1164.all; use ieee.numeric_std.all;
entity sb_spram256ka_tb is end entity;
architecture sim of sb_spram256ka_tb is
  signal clk : std_logic := '0';
  signal din, dout : std_logic_vector(15 downto 0) := (others => '0');
  signal addr : std_logic_vector(13 downto 0) := (others => '0');
  signal mask : std_logic_vector(3 downto 0) := (others => '0');
  signal wren, cs : std_logic := '0';
  signal done : boolean := false;
begin
  uut : entity work.SB_SPRAM256KA
    port map (DATAIN=>din, ADDRESS=>addr, MASKWREN=>mask, WREN=>wren,
              CHIPSELECT=>cs, CLOCK=>clk, STANDBY=>'0', SLEEP=>'0', POWEROFF=>'1',
              DATAOUT=>dout);
  clk <= not clk after 5 ns when not done else '0';
  stim : process
    procedure tick is begin wait until rising_edge(clk); wait for 1 ns; end procedure;
  begin
    cs <= '1';
    -- write 0xBEEF at addr 5
    addr <= std_logic_vector(to_unsigned(5,14)); din <= x"BEEF"; mask <= "1111"; wren <= '1'; tick;
    -- read back: present addr, no write; DATAOUT valid the cycle after
    wren <= '0'; mask <= "0000"; addr <= std_logic_vector(to_unsigned(5,14)); tick; tick;
    assert dout = x"BEEF" report "read-back failed" severity failure;
    -- masked write nibble 1 only: 0x_A_ -> 0xBEAF
    addr <= std_logic_vector(to_unsigned(5,14)); din <= x"00A0"; mask <= "0010"; wren <= '1'; tick;
    wren <= '0'; mask <= "0000"; addr <= std_logic_vector(to_unsigned(5,14)); tick; tick;
    assert dout = x"BEAF" report "masked write failed" severity failure;
    report "sb_spram256ka_tb PASSED" severity note;
    done <= true; wait;
  end process;
end architecture;
