-------------------------------------------------------------------------------
-- qspi_flash_model_tb.vhd
--
-- Self-check testbench for qspi_flash_model. Bit-bangs the model directly
-- (acting as the "controller") for:
--   1) a 0x0B single-SPI Fast-Read of address 0x000010
--   2) a 0xEB Quad-I/O Fast-Read of address 0x000040
-- and checks the returned bytes against the model's deterministic pattern
--   byte(addr) = addr(7 downto 0) xor addr(15 downto 8)
-- for several bytes, including auto-increment across the burst.
-------------------------------------------------------------------------------

library ieee;
use ieee.std_logic_1164.all;
use ieee.numeric_std.all;

entity qspi_flash_model_tb is
end entity;

architecture tb of qspi_flash_model_tb is

  constant T : time := 20 ns; -- sck half period

  signal cs_n  : std_logic := '1';
  signal sck   : std_logic := '0';
  signal io_i  : std_logic_vector(3 downto 0) := "0000";
  signal io_oe : std_logic_vector(3 downto 0) := "0000"; -- controller drive enables
  signal io_o  : std_logic_vector(3 downto 0);           -- model-driven lines

  signal test_done : boolean := false;

  -- reference pattern function, matches the model's documented convention
  function byte_of_addr(addr : std_logic_vector(23 downto 0)) return std_logic_vector is
  begin
    return addr(7 downto 0) xor addr(15 downto 8);
  end function;

  -- drive one SCK pulse (mode 0: data set up before rising edge, model
  -- samples on rising edge, drives on falling edge)
  procedure sck_pulse(signal sck : inout std_logic) is
  begin
    wait for T;
    sck <= '1';
    wait for T;
    sck <= '0';
  end procedure;

begin

  dut : entity work.qspi_flash_model
    port map (
      cs_n  => cs_n,
      sck   => sck,
      io_i  => io_i,
      io_oe => io_oe,
      io_o  => io_o);

  stim : process
    variable addr : unsigned(23 downto 0);
    variable exp  : std_logic_vector(7 downto 0);
    variable got  : std_logic_vector(7 downto 0);
  begin
    ---------------------------------------------------------------------
    -- Test 1: 0x0B single-SPI Fast-Read of address 0x000010
    ---------------------------------------------------------------------
    cs_n  <= '1';
    io_oe <= "0000";
    io_i  <= "0000";
    wait for T;

    cs_n <= '0';
    io_oe <= "0001"; -- controller drives IO0 (MOSI) during cmd/addr

    -- command byte 0x0B, MSB first, on IO0
    for i in 7 downto 0 loop
      io_i(0) <= std_logic(to_unsigned(16#0B#, 8)(i));
      sck_pulse(sck);
    end loop;

    -- 24-bit address 0x000010, MSB first, on IO0
    addr := to_unsigned(16#000010#, 24);
    for i in 23 downto 0 loop
      io_i(0) <= addr(i);
      sck_pulse(sck);
    end loop;

    -- 8 dummy clocks
    io_i(0) <= '0';
    for i in 0 to 7 loop
      sck_pulse(sck);
    end loop;

    -- data phase: controller must tristate (release IO0/IO1)
    io_oe <= "0000";

    -- read several bytes, MSB first on IO1 (MISO), checking auto-increment
    for byte_idx in 0 to 2 loop
      got := (others => '0');
      for i in 7 downto 0 loop
        wait for T;
        sck <= '1';
        got(i) := io_o(1);
        wait for T;
        sck <= '0';
      end loop;
      exp := byte_of_addr(std_logic_vector(addr + byte_idx));
      assert got = exp
        report "0x0B single-read: byte mismatch at index " &
               integer'image(byte_idx)
        severity failure;
    end loop;

    cs_n <= '1';
    wait for T;

    ---------------------------------------------------------------------
    -- Test 2: 0xEB Quad-I/O Fast-Read of address 0x000040
    ---------------------------------------------------------------------
    io_oe <= "0000";
    io_i  <= "0000";
    wait for T;

    cs_n <= '0';
    io_oe <= "0001"; -- command byte still single-SPI on IO0

    for i in 7 downto 0 loop
      io_i(0) <= std_logic(to_unsigned(16#EB#, 8)(i));
      sck_pulse(sck);
    end loop;

    -- 24-bit address, QUAD (IO0-3), 4 bits/clock, MSB first -> 6 clocks
    addr := to_unsigned(16#000040#, 24);
    io_oe <= "1111";
    for i in 5 downto 0 loop
      io_i(3) <= addr(i*4 + 3);
      io_i(2) <= addr(i*4 + 2);
      io_i(1) <= addr(i*4 + 1);
      io_i(0) <= addr(i*4 + 0);
      sck_pulse(sck);
    end loop;

    -- mode byte + dummy: 6 quad clocks total (model's documented convention)
    io_i <= "0000";
    for i in 0 to 5 loop
      sck_pulse(sck);
    end loop;

    -- data phase: controller must tristate all four lines
    io_oe <= "0000";

    for byte_idx in 0 to 2 loop
      got := (others => '0');
      for i in 1 downto 0 loop -- 2 nibbles per byte, MSB nibble first
        wait for T;
        sck <= '1';
        got(i*4+3) := io_o(3);
        got(i*4+2) := io_o(2);
        got(i*4+1) := io_o(1);
        got(i*4+0) := io_o(0);
        wait for T;
        sck <= '0';
      end loop;
      exp := byte_of_addr(std_logic_vector(addr + byte_idx));
      assert got = exp
        report "0xEB quad-read: byte mismatch at index " &
               integer'image(byte_idx)
        severity failure;
    end loop;

    cs_n <= '1';

    report "PASSED";
    test_done <= true;
    wait;
  end process;

end architecture;
