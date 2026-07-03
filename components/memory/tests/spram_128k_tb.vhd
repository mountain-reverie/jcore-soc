library ieee; use ieee.std_logic_1164.all; use ieee.numeric_std.all;
entity spram_128k_tb is end entity;
architecture sim of spram_128k_tb is
  signal clk : std_logic := '0';
  signal en  : std_logic := '0';
  signal we  : std_logic_vector(3 downto 0) := (others => '0');
  signal a   : std_logic_vector(16 downto 2) := (others => '0');
  signal dw  : std_logic_vector(31 downto 0) := (others => '0');
  signal dr  : std_logic_vector(31 downto 0);
  signal done : boolean := false;

  constant BANK0_ADDR : std_logic_vector(16 downto 2) := std_logic_vector(to_unsigned(5, 15));
  constant BANK1_ADDR : std_logic_vector(16 downto 2) := std_logic_vector(to_unsigned(16384 + 7, 15));
  constant MASK_ADDR  : std_logic_vector(16 downto 2) := std_logic_vector(to_unsigned(10, 15));
begin
  uut : entity work.spram_128k
    port map (clk=>clk, en=>en, we=>we, a=>a, dw=>dw, dr=>dr);

  clk <= not clk after 5 ns when not done else '0';

  stim : process
    procedure tick is begin wait until rising_edge(clk); wait for 1 ns; end procedure;
  begin
    en <= '1';

    -- write distinct values into bank0 and bank1 words
    a <= BANK0_ADDR; dw <= x"11223344"; we <= "1111"; tick;
    a <= BANK1_ADDR; dw <= x"55667788"; we <= "1111"; tick;

    -- read back bank0 (present addr, no write; result valid N+1)
    we <= "0000"; a <= BANK0_ADDR; tick; tick;
    assert dr = x"11223344" report "bank0 read-back failed" severity failure;

    -- read back bank1
    we <= "0000"; a <= BANK1_ADDR; tick; tick;
    assert dr = x"55667788" report "bank1 read-back failed" severity failure;

    -- write a known word for byte-masked test
    a <= MASK_ADDR; dw <= x"AABBCCDD"; we <= "1111"; tick;

    -- byte-masked write: we="0100" writes only byte2 (dw(23:16))
    a <= MASK_ADDR; dw <= x"00990000"; we <= "0100"; tick;

    -- read back and verify only byte2 changed
    we <= "0000"; a <= MASK_ADDR; tick; tick;
    assert dr = x"AA99CCDD" report "byte-masked write failed" severity failure;

    report "spram_128k_tb PASSED" severity note;
    done <= true; wait;
  end process;
end architecture;
