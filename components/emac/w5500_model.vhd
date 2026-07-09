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
    sipr_out : out std_logic_vector(31 downto 0);

    -- Extended (Task 8b) outputs, backward-compatible additions: capture of
    -- socket-0 TX buffer writes (BSB=0x02) and a one-cycle-wide pulse when
    -- Sn_CR (BSB=0x01, addr 0x0001) is written with SEND (0x20). Existing
    -- instantiations (e.g. icesugar_top_tb.vhd) that don't connect these
    -- new output ports are unaffected.
    sock0_tx_bytes : out std_logic_vector(191 downto 0) := (others => '0'); -- 24 bytes, byte0 = bits(191 downto 184)
    sent           : out std_logic := '0'
  );
end entity;

architecture sim of w5500_model is
  type reg_mem_t is array (0 to 18) of std_logic_vector(7 downto 0);
  signal reg_mem : reg_mem_t := (others => (others => '0'));

  -- Socket-0 register block (BSB=0x01) and TX buffer (BSB=0x02), modeled
  -- separately from the common block so common-block addresses (e.g. GAR at
  -- 0x0001) don't alias with socket-0 register addresses (e.g. Sn_CR at
  -- 0x0001).
  type sock0_reg_t is array (0 to 63) of std_logic_vector(7 downto 0);
  signal sock0_reg : sock0_reg_t := (others => (others => '0'));
  type tx_buf_t is array (0 to 2047) of std_logic_vector(7 downto 0);
  signal tx_buf : tx_buf_t := (others => (others => '0'));

  constant BSB_COMMON    : integer := 0;
  constant BSB_SOCK0_REG : integer := 1;
  constant BSB_SOCK0_TX  : integer := 2;
begin

  shar_out <= reg_mem(9) & reg_mem(10) & reg_mem(11) & reg_mem(12) & reg_mem(13) & reg_mem(14);
  sipr_out <= reg_mem(15) & reg_mem(16) & reg_mem(17) & reg_mem(18);

  spi_proc : process
    variable addr_hi, addr_lo, ctrl : std_logic_vector(7 downto 0);
    variable addr    : integer;
    variable bsb     : integer;
    variable is_write : boolean;
    variable shreg   : std_logic_vector(7 downto 0);
    variable bitcnt  : integer;
    variable byte_i  : integer;
  begin
    -- Wait for a frame to begin (cs falling edge).
    wait until spi_cs = '0';
    sent <= '0';

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
      bsb := to_integer(unsigned(ctrl(7 downto 3)));

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
            -- Read: present each bit MSB-first and advance on the master's
            -- SAMPLE edge (sclk rising, spi2 cpha=0 samples miso there). The
            -- header loop above exits on a rising edge with sclk HIGH, so
            -- waiting on the next rising here naturally skips the control
            -- byte's trailing falling edge -- driving on the falling edge
            -- instead would consume that trailing edge as a bit boundary and
            -- shift every read byte by one bit.
            if bitcnt = 0 then
              if bsb = BSB_COMMON and addr >= 0 and addr <= 18 then
                shreg := reg_mem(addr);
              elsif bsb = BSB_SOCK0_REG and addr >= 0 and addr <= 63 then
                shreg := sock0_reg(addr);
              else
                shreg := (others => '0');
              end if;
            end if;
            spi_miso <= shreg(7);
            wait until (spi_sclk = '1' and spi_sclk'event) or spi_cs = '1';
            exit data_bytes when spi_cs = '1';
            shreg := shreg(6 downto 0) & '0';
          end if;
          bitcnt := bitcnt + 1;
        end loop;

        if is_write then
          if bsb = BSB_COMMON and addr >= 0 and addr <= 18 then
            reg_mem(addr) <= shreg;
          elsif bsb = BSB_SOCK0_REG and addr >= 0 and addr <= 63 then
            sock0_reg(addr) <= shreg;
            -- Model the chip's autonomous socket-0 reactions so the
            -- driver's polling loops (eth_init/send_once in eth_report.c)
            -- make progress, mirroring eth_report.c's HOST_TEST model.
            if addr = 1 then                      -- Sn_CR
              if shreg = x"01" then                -- OPEN
                sock0_reg(3) <= x"22";              -- Sn_SR = SOCK_UDP
              elsif shreg = x"20" then              -- SEND
                sock0_reg(2) <= sock0_reg(2) or x"10"; -- Sn_IR.SENDOK
                sent <= '1';
                for k in 0 to 23 loop
                  sock0_tx_bytes(191 - k*8 downto 184 - k*8) <= tx_buf(k);
                end loop;
              end if;
            end if;
          elsif bsb = BSB_SOCK0_TX and addr >= 0 and addr <= 2047 then
            tx_buf(addr mod 2048) <= shreg;
          end if;
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
