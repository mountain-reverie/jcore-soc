library ieee;
use ieee.std_logic_1164.all;
use ieee.numeric_std.all;

entity spram_128k is
  port (
    clk : in  std_logic;
    en  : in  std_logic;
    we  : in  std_logic_vector(3 downto 0);   -- byte write enables
    a   : in  std_logic_vector(16 downto 2);  -- 32-bit word address (bit16=bank)
    dw  : in  std_logic_vector(31 downto 0);
    dr  : out std_logic_vector(31 downto 0));
end entity;

architecture rtl of spram_128k is
  component SB_SPRAM256KA is
    port (
      DATAIN     : in  std_logic_vector(15 downto 0);
      ADDRESS    : in  std_logic_vector(13 downto 0);
      MASKWREN   : in  std_logic_vector(3 downto 0);
      WREN       : in  std_logic;
      CHIPSELECT : in  std_logic;
      CLOCK      : in  std_logic;
      STANDBY    : in  std_logic;
      SLEEP      : in  std_logic;
      POWEROFF   : in  std_logic;
      DATAOUT    : out std_logic_vector(15 downto 0));
  end component;

  signal word_a : std_logic_vector(13 downto 0);
  signal bank   : std_logic;
  -- per-bank chip selects and per-16-bit-half MASKWREN
  signal cs0, cs1 : std_logic;
  -- byte we -> nibble MASKWREN: byte0->MASKWREN(1:0), byte1->MASKWREN(3:2)
  signal mask_lo, mask_hi : std_logic_vector(3 downto 0);
  -- per-half WREN: asserted only for an actual write to that half
  signal wren_lo, wren_hi : std_logic;
  signal dout0_lo, dout0_hi, dout1_lo, dout1_hi : std_logic_vector(15 downto 0);
  signal bank_r : std_logic;
begin
  word_a <= a(15 downto 2);
  bank   <= a(16);
  cs0 <= en and not bank;
  cs1 <= en and bank;
  -- low half = bytes 0,1 ; high half = bytes 2,3
  mask_lo <= (we(1), we(1), we(0), we(0));
  mask_hi <= (we(3), we(3), we(2), we(2));
  wren_lo <= we(0) or we(1);
  wren_hi <= we(2) or we(3);

  -- Bank 0
  b0_lo : SB_SPRAM256KA port map (
    DATAIN=>dw(15 downto 0), ADDRESS=>word_a, MASKWREN=>mask_lo, WREN=>wren_lo,
    CHIPSELECT=>cs0, CLOCK=>clk, STANDBY=>'0', SLEEP=>'0', POWEROFF=>'1', DATAOUT=>dout0_lo);
  b0_hi : SB_SPRAM256KA port map (
    DATAIN=>dw(31 downto 16), ADDRESS=>word_a, MASKWREN=>mask_hi, WREN=>wren_hi,
    CHIPSELECT=>cs0, CLOCK=>clk, STANDBY=>'0', SLEEP=>'0', POWEROFF=>'1', DATAOUT=>dout0_hi);
  -- Bank 1
  b1_lo : SB_SPRAM256KA port map (
    DATAIN=>dw(15 downto 0), ADDRESS=>word_a, MASKWREN=>mask_lo, WREN=>wren_lo,
    CHIPSELECT=>cs1, CLOCK=>clk, STANDBY=>'0', SLEEP=>'0', POWEROFF=>'1', DATAOUT=>dout1_lo);
  b1_hi : SB_SPRAM256KA port map (
    DATAIN=>dw(31 downto 16), ADDRESS=>word_a, MASKWREN=>mask_hi, WREN=>wren_hi,
    CHIPSELECT=>cs1, CLOCK=>clk, STANDBY=>'0', SLEEP=>'0', POWEROFF=>'1', DATAOUT=>dout1_hi);

  -- WREN must be asserted ONLY for a real write, not for every enabled cycle.
  -- Driving WREN=en was safe against corrupting memory (a read has we=0000 ->
  -- MASKWREN 0 -> no nibble written), but on real SB_SPRAM256KA silicon a read
  -- requires WREN=0: with WREN=1 the block neither writes nor returns valid
  -- DATAOUT, so every read came back as zeros on hardware while the behavioral
  -- model -- which updates DATAOUT regardless of WREN -- read back fine.
  -- Register the bank to mux read-back at N+1.
  process (clk) is begin
    if rising_edge(clk) then bank_r <= bank; end if;
  end process;

  dr <= (dout1_hi & dout1_lo) when bank_r = '1' else (dout0_hi & dout0_lo);
end architecture;
