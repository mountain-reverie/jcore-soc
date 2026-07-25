-- RAM with 1 read/write port, sync reads and writes, 16 bits wide with 2 8-bit
-- byte write select inputs, and 64 entries deep.
library ieee;
use ieee.std_logic_1164.all;
entity ram_2x8x64_1rw is
  port (
    rst : in  std_logic;
    clk : in  std_logic;
    en  : in  std_logic;
    wr  : in  std_logic;
    we  : in  std_logic_vector( 1 downto 0);
    a   : in  std_logic_vector( 5 downto 0);
    dw  : in  std_logic_vector(15 downto 0);
    dr  : out std_logic_vector(15 downto 0);
    margin : in std_logic_vector(1 downto 0));
end entity;
