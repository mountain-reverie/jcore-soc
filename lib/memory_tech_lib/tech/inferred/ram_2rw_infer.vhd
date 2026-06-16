-- Directly implement memory using a VHDL array. Used either in simulation or to
-- infer block RAM (Xilinx XST and, for the ECP5 boards, the ghdl-yosys flow).
--
-- Single-clock dual-port form: both ports are clocked by clk0. This is correct
-- for every FPGA consumer because the cache clock-domain crossing is collapsed
-- in the *_fpga ddr_ram_mux configurations -- clk0 and clk1 are always driven by
-- the same net (mimas/turtle: clk_sys; ulx3s: clk_cpu; the 125/200 MHz split is
-- an ASIC-era artifact). ghdl --synth requires a single process to write the
-- memory (two writer processes raise "multiple assignments for mem"), so a true
-- two-clock variant is not inferable through the ghdl-yosys frontend; XST infers
-- a same-clock true-dual-port BRAM from this single-process template equally.
-- clk1/rst1/margin* are intentionally unused.
architecture inferred of ram_2rw is
  type mem_t is array (integer range 0 to 2**ADDR_WIDTH - 1)
    of std_logic_vector(dw0'length - 1 downto 0);
  signal wr_we0 : std_logic_vector(SUBWORD_NUM - 1 downto 0);
  signal wr_we1 : std_logic_vector(SUBWORD_NUM - 1 downto 0);
begin
  one_subword : if SUBWORD_NUM = 1 generate
    -- both ports share one process (and one memory variable) on clk0
    process(clk0)
      variable mem : mem_t;
    begin
      if clk0'event and clk0 = '1' then
        if en0 = '1' then
          if wr0 = '1' then
            mem(to_integer(unsigned(a0))) := dw0;   -- synchronous write
          else
            dr0 <= mem(to_integer(unsigned(a0)));   -- synchronous latched read
          end if;
        end if;
        if en1 = '1' then
          if wr1 = '1' then
            mem(to_integer(unsigned(a1))) := dw1;
          else
            dr1 <= mem(to_integer(unsigned(a1)));
          end if;
        end if;
      end if;
    end process;
  end generate;
  subwords : if SUBWORD_NUM /= 1 generate
    -- combine wr and we so the tools infer a byte-write-enabled block RAM
    wr_we0 <= mask_bits(wr0, we0);
    wr_we1 <= mask_bits(wr1, we1);

    process(clk0)
      variable mem : mem_t;
    begin
      if clk0'event and clk0 = '1' then
        if en0 = '1' then
          if wr_we0 = (wr_we0'range => '0') then
            dr0 <= mem(to_integer(unsigned(a0)));   -- synchronous latched read
          end if;
          for i in integer range 0 to SUBWORD_NUM-1 loop  -- synchronous write
            if wr_we0(i) = '1' then
              mem(to_integer(unsigned(a0)))((i+1)*SUBWORD_WIDTH-1 downto i*SUBWORD_WIDTH)
                := dw0((i+1)*SUBWORD_WIDTH-1 downto i*SUBWORD_WIDTH);
            end if;
          end loop;
        end if;
        if en1 = '1' then
          if wr_we1 = (wr_we1'range => '0') then
            dr1 <= mem(to_integer(unsigned(a1)));
          end if;
          for i in integer range 0 to SUBWORD_NUM-1 loop
            if wr_we1(i) = '1' then
              mem(to_integer(unsigned(a1)))((i+1)*SUBWORD_WIDTH-1 downto i*SUBWORD_WIDTH)
                := dw1((i+1)*SUBWORD_WIDTH-1 downto i*SUBWORD_WIDTH);
            end if;
          end loop;
        end if;
      end if;
    end process;
  end generate;
end architecture;
