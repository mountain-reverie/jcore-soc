library ieee;
use ieee.std_logic_1164.all;
use ieee.numeric_std.all;

package sdram_pkg is
  -- Geometry (ULX3S 32MB 16-bit SDR SDRAM defaults).
  constant SDR_ROW_BITS  : integer := 13;
  constant SDR_COL_BITS  : integer := 9;
  constant SDR_BANK_BITS : integer := 2;

  -- Downward command bus (single-edge SDRAM pins; A is the muxed row/col bus).
  type sdram_cmd_t is record
    cke   : std_logic;
    cs_n  : std_logic;
    ras_n : std_logic;
    cas_n : std_logic;
    we_n  : std_logic;
    ba    : std_logic_vector(SDR_BANK_BITS - 1 downto 0);
    a     : std_logic_vector(SDR_ROW_BITS - 1 downto 0);
    dqm   : std_logic_vector(1 downto 0);
  end record;

  -- Command encodings (cs_n,ras_n,cas_n,we_n), CKE assumed '1'.
  constant CMD_NOP   : std_logic_vector(3 downto 0) := "0111";
  constant CMD_ACT   : std_logic_vector(3 downto 0) := "0011";
  constant CMD_READ  : std_logic_vector(3 downto 0) := "0101";
  constant CMD_WRITE : std_logic_vector(3 downto 0) := "0100";
  constant CMD_PRE   : std_logic_vector(3 downto 0) := "0010";
  constant CMD_REF   : std_logic_vector(3 downto 0) := "0001";
  constant CMD_LMR   : std_logic_vector(3 downto 0) := "0000";
  constant CMD_DESL  : std_logic_vector(3 downto 0) := "1111";

  -- Decompose a 32-bit byte address into bank/row/col (16-bit-word geometry).
  -- Word address = a(31 downto 1); a(0) is ignored (16-bit data bus, dqm masks
  -- bytes). Layout from LSB of the word address: [col][bank][row].
  type sdram_addr_t is record
    bank : std_logic_vector(SDR_BANK_BITS - 1 downto 0);
    row  : std_logic_vector(SDR_ROW_BITS - 1 downto 0);
    col  : std_logic_vector(SDR_COL_BITS - 1 downto 0);
  end record;
  function sdram_addr(a : std_logic_vector(31 downto 0)) return sdram_addr_t;
end package;

package body sdram_pkg is
  function sdram_addr(a : std_logic_vector(31 downto 0)) return sdram_addr_t is
    variable r : sdram_addr_t;
    constant CL : integer := SDR_COL_BITS;
    constant BL : integer := SDR_BANK_BITS;
    constant RL : integer := SDR_ROW_BITS;
  begin
    r.col  := a(1 + CL - 1 downto 1);
    r.bank := a(1 + CL + BL - 1 downto 1 + CL);
    r.row  := a(1 + CL + BL + RL - 1 downto 1 + CL + BL);
    return r;
  end function;
end package body;
