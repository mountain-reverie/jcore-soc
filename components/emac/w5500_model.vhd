-- Behavioral SPI-slave model of a W5500 (WIZ850io), sim-only. Implements
-- just enough of the common register block (BSB=0) to let a testbench
-- verify that a driver correctly programs MAC/IP/subnet/gateway: it shifts
-- in SPI frames (addr_hi, addr_lo, control, data...) while cs is low and,
-- for writes (control bit2 = RWB = '1'), stores the data bytes into
-- reg_mem indexed directly by the low byte of the register address (the
-- whole common block 0x0000-0x0012 fits in 0 to 18). For reads it shifts
-- reg_mem content back out on miso, MSB-first, on SCLK falling edges.
--
-- SPI mode: matches spi2's cpol='0'/cpha='0' (mode 0) instantiation for the
-- icesugar 'eth' device -- sample mosi on SCLK rising edges, drive miso
-- following SCLK falling edges (and immediately after cs goes low, for the
-- first bit).
library ieee;
use ieee.std_logic_1164.all;
use ieee.numeric_std.all;

entity w5500_model is
  port (
    clk      : in  std_logic;   -- unused by the SPI shift logic; present for
                                 -- interface symmetry / possible future use.
    spi_sclk : in  std_logic;
    spi_mosi : in  std_logic;
    spi_miso : out std_logic;
    spi_cs   : in  std_logic;   -- active low
    -- Derived/captured outputs for the testbench to inspect directly
    -- (VHDL-93 has no external-name access into an instance, so these are
    -- plain ports the tb wires up).
    shar_out : out std_logic_vector(47 downto 0);
    sipr_out : out std_logic_vector(31 downto 0)
  );
end entity;

architecture sim of w5500_model is
  type reg_mem_t is array (0 to 18) of std_logic_vector(7 downto 0);
  signal reg_mem : reg_mem_t := (others => (others => '0'));
begin

  shar_out <= reg_mem(9) & reg_mem(10) & reg_mem(11) & reg_mem(12) & reg_mem(13) & reg_mem(14);
  sipr_out <= reg_mem(15) & reg_mem(16) & reg_mem(17) & reg_mem(18);

  spi_proc : process
    variable addr_hi, addr_lo, ctrl : std_logic_vector(7 downto 0);
    variable addr    : integer;
    variable is_write : boolean;
    variable shreg   : std_logic_vector(7 downto 0);
    variable bitcnt  : integer;
    variable byte_i  : integer;
  begin
    -- Wait for a frame to begin (cs falling edge).
    wait until spi_cs = '0';

    -- Shift in three header bytes MSB-first, sampling mosi on sclk rising
    -- edges, aborting early if cs is deasserted mid-frame.
    byte_i := 0;
    while byte_i < 3 loop
      shreg := (others => '0');
      bitcnt := 0;
      while bitcnt < 8 loop
        wait until (spi_sclk = '1' and spi_sclk'event) or spi_cs = '1';
        if spi_cs = '1' then exit; end if;
        shreg := shreg(6 downto 0) & spi_mosi;
        bitcnt := bitcnt + 1;
      end loop;
      exit when spi_cs = '1';
      case byte_i is
        when 0 => addr_hi := shreg;
        when 1 => addr_lo := shreg;
        when others => ctrl := shreg;
      end case;
      byte_i := byte_i + 1;
    end loop;

    if spi_cs = '0' then
      addr := to_integer(unsigned(addr_hi)) * 256 + to_integer(unsigned(addr_lo));
      is_write := ctrl(2) = '1';

      -- Shift subsequent data bytes until cs is deasserted.
      data_bytes : loop
        shreg := (others => '0');
        bitcnt := 0;
        while bitcnt < 8 loop
          if is_write then
            wait until (spi_sclk = '1' and spi_sclk'event) or spi_cs = '1';
            exit data_bytes when spi_cs = '1';
            shreg := shreg(6 downto 0) & spi_mosi;
          else
            -- Drive miso following sclk falling edges, MSB-first.
            if bitcnt = 0 then
              if addr >= 0 and addr <= 18 then
                shreg := reg_mem(addr);
              else
                shreg := (others => '0');
              end if;
            end if;
            spi_miso <= shreg(7);
            wait until (spi_sclk = '0' and spi_sclk'event) or spi_cs = '1';
            exit data_bytes when spi_cs = '1';
            shreg := shreg(6 downto 0) & '0';
          end if;
          bitcnt := bitcnt + 1;
        end loop;

        if is_write and addr >= 0 and addr <= 18 then
          reg_mem(addr) <= shreg;
        end if;
        addr := addr + 1;
      end loop data_bytes;
    end if;

    -- Frame ends on cs rising edge; wait for it if we haven't seen it yet.
    if spi_cs = '0' then
      wait until spi_cs = '1';
    end if;
  end process;

end architecture;
