library ieee;
use ieee.std_logic_1164.all;
use ieee.numeric_std.all;
use work.cpu2j0_pack.all;
use work.data_bus_pack.all;
use work.cpu_core_pack.all;
use work.xip_flash_pkg.all;

-- Task 7: full end-to-end XIP demand-paging cosim. Elaborates cpus(one_cpu_xip)
-- (Task 5) the same way the Task 5/6 smoke test did, but now:
--  * loads the real xip_boot.bin resident image (crt0 + _pf_handler +
--    round-robin victim counter, Task 6) via bootram_infer/boot_image_pkg
--    (packed by sim.sh, same mechanism cpus_one_ebr uses for its own boot
--    image -- see sim.sh's "xip" branch);
--  * wires a behavioral SPI flash model (Fast-Read 0x0B protocol, same shape
--    as spi_page_cache_tb.vhd's pc_flash_model / t1_fill flash_model) to the
--    REAL pad-level pins (pin_cs_n/pin_sck/pin_mosi/pin_miso) that
--    cpus_xip.vhd's ice_spi_io/SB_IO instantiation drives -- reached via
--    VHDL-2008 external names (`<<signal ...>>`), since the generated `cpus`
--    entity (targets/cpus.vhd) exposes no flash pin ports at all (see Task 5's
--    report) and this task's brief forbids hand-editing that generated
--    entity. The external-name pathname is `.cpus_xip_tb.dut.<signal>`,
--    where `dut` is this tb's `cpus` component instance label and
--    `pin_cs_n`/`pin_sck`/`pin_mosi`/`pin_miso` are cpus_xip.vhd's internal
--    (non-port) signals one level inside that instance's architecture
--    (one_cpu_xip). This file must therefore be analyzed under --std=08 (only
--    this file -- everything else in the design stays --std=93, external
--    names work across the boundary since the referenced object's std
--    version is irrelevant to the referencing pathname).
--  * loaded flash image is xip_test.img (>4-page test program + table),
--    packed into a byte-array VHDL package (xip_flash_pkg, via
--    tools/genflashpkg) at flash offset FLASH_IMG_BASE = 0x100000, matching
--    xip.x's FLASH_BASE;
--  * releases reset and runs until the CPU writes 0 to TEST_RESULT_ADDRESS
--    (0xBCDE0010, the components/cpu/sim/tests/sim_instr.h convention Task 6
--    used) on the DATA MASTER bus -- watched via the `data_master_snoop`
--    external name (cpus_xip.vhd's pre-decode data-master tap, fed into
--    spi_page_cache's window/fault decode) rather than any device bus slot,
--    since 0xBCDE0010's top nibble (0xB) is NOT one of decode_core_data_addr's
--    recognized prefixes (x"0"/x"1"/x"a") and so a store to it is silently
--    swallowed as DEV_NONE *inside* cpu_core (same class of "dead end inside
--    cpu_core" issue Task 5's report flagged for the old PC_WIN_TAG=0x4
--    placement) -- it can only be observed on the raw master bus, which
--    data_master_snoop is by construction (Task 4's pre-decode CPU tap).
entity cpus_xip_tb is end entity;

architecture sim of cpus_xip_tb is
  constant CLK_PER : time := 20 ns; -- ~50 MHz free-running sim clock

  signal clk : std_logic := '0';
  signal rst : std_logic := '1';
  signal done : boolean := false;

  signal cpu0_periph_dbus_o : cpu_data_o_t;
  signal cpu0_periph_dbus_i : cpu_data_i_t;
  signal cpu0_ddr_ibus_o : cpu_instruction_o_t;
  signal cpu0_ddr_ibus_i : cpu_instruction_i_t;
  signal cpu0_ddr_dbus_o : cpu_data_o_t;
  signal cpu0_ddr_dbus_i : cpu_data_i_t;
  signal cpu0_mem_lock : std_logic;

  signal cpu1_periph_dbus_o : cpu_data_o_t;
  signal cpu1_periph_dbus_i : cpu_data_i_t := (d => (others => '0'), ack => '0');
  signal cpu1_ddr_ibus_o : cpu_instruction_o_t;
  signal cpu1_ddr_ibus_i : cpu_instruction_i_t := (d => (others => '0'), ack => '0');
  signal cpu1_ddr_dbus_o : cpu_data_o_t;
  signal cpu1_ddr_dbus_i : cpu_data_i_t := (d => (others => '0'), ack => '0');
  signal cpu1_mem_lock : std_logic;

  signal debug_o : cpu_debug_o_t;
  signal cpu0_data_master_en : std_logic;
  signal cpu1_data_master_en : std_logic;
  signal cpu0_data_master_ack : std_logic;
  signal cpu1_data_master_ack : std_logic;

  signal cpu0_event_o : cpu_event_o_t;
  signal cpu1_event_o : cpu_event_o_t;

  signal cpu0_copro_o : cop_o_t;
  signal cpu1_copro_o : cop_o_t;

  constant TEST_RESULT_ADDRESS : std_logic_vector(31 downto 0) := x"BCDE0010";

  signal test_result_seen : boolean := false;
  signal test_result_val  : std_logic_vector(31 downto 0) := (others => '1');

  component cpus is
    generic (
      INSERT_WRITE_DELAY_BOOT_MEM : boolean;
      INSERT_READ_DELAY_BOOT_MEM : boolean;
      INSERT_INST_DELAY_BOOT_MEM : boolean);
    port (
      clk : in std_logic;
      rst : in std_logic;
      cpu0_periph_dbus_o : out cpu_data_o_t;
      cpu0_periph_dbus_i : in  cpu_data_i_t;
      cpu0_ddr_ibus_o : out cpu_instruction_o_t;
      cpu0_ddr_ibus_i : in  cpu_instruction_i_t;
      cpu0_ddr_dbus_o : out cpu_data_o_t;
      cpu0_ddr_dbus_i : in cpu_data_i_t;
      cpu0_mem_lock : out std_logic;
      cpu1_periph_dbus_o : out cpu_data_o_t;
      cpu1_periph_dbus_i : in cpu_data_i_t;
      cpu1_ddr_ibus_o : out cpu_instruction_o_t;
      cpu1_ddr_ibus_i : in  cpu_instruction_i_t;
      cpu1_ddr_dbus_o : out cpu_data_o_t;
      cpu1_ddr_dbus_i : in  cpu_data_i_t;
      cpu1_mem_lock : out std_logic;
      debug_i : in  cpu_debug_i_t;
      debug_o : out cpu_debug_o_t;
      cpu0_data_master_en : out std_logic;
      cpu1_data_master_en : out std_logic;
      cpu0_data_master_ack : out std_logic;
      cpu1_data_master_ack : out std_logic;
      cpu1eni : in std_logic;
      cpu0_event_o : out cpu_event_o_t;
      cpu0_event_i : in cpu_event_i_t;
      cpu1_event_o : out cpu_event_o_t;
      cpu1_event_i : in cpu_event_i_t;
      cpu0_copro_o : out cop_o_t;
      cpu0_copro_i : in  cop_i_t;
      cpu1_copro_o : out cop_o_t;
      cpu1_copro_i : in  cop_i_t;
      -- Digital config-flash SPI ports (Task 8 refactor): the page cache's
      -- fill-engine SPI master, brought out of cpus for the padring to map to
      -- the real SB_IO cell. This cosim wires a behavioral flash straight to
      -- these digital ports (no SB_IO/pad model needed).
      spi_d_cs_n : out std_logic;
      spi_d_sck : out std_logic;
      spi_d_mosi : out std_logic;
      spi_d_miso : in std_logic);
  end component;

  -- Digital SPI between cpus (master) and the behavioral flash model below.
  signal spi_d_cs_n : std_logic;
  signal spi_d_sck  : std_logic;
  signal spi_d_mosi : std_logic;
  signal spi_d_miso : std_logic := '0';

begin

  clk <= not clk after CLK_PER / 2 when not done else '0';

  -- Loop back the buses nothing external drives in this cosim: DEV_PERIPH and
  -- DEV_DDR (external) masters get an ack'd, all-zero-data reply so an
  -- incidental access doesn't wedge the CPU (matches devices.vhd / an
  -- unpopulated external DDR).
  cpu0_periph_dbus_i <= loopback_bus(cpu0_periph_dbus_o);
  cpu0_ddr_ibus_i <= loopback_bus(cpu0_ddr_ibus_o);
  cpu0_ddr_dbus_i <= loopback_bus(cpu0_ddr_dbus_o);

  dut : cpus
    generic map (
      INSERT_WRITE_DELAY_BOOT_MEM => false,
      INSERT_READ_DELAY_BOOT_MEM => false,
      INSERT_INST_DELAY_BOOT_MEM => false)
    port map (
      clk => clk, rst => rst,
      cpu0_periph_dbus_o => cpu0_periph_dbus_o, cpu0_periph_dbus_i => cpu0_periph_dbus_i,
      cpu0_ddr_ibus_o => cpu0_ddr_ibus_o, cpu0_ddr_ibus_i => cpu0_ddr_ibus_i,
      cpu0_ddr_dbus_o => cpu0_ddr_dbus_o, cpu0_ddr_dbus_i => cpu0_ddr_dbus_i,
      cpu0_mem_lock => cpu0_mem_lock,
      cpu1_periph_dbus_o => cpu1_periph_dbus_o, cpu1_periph_dbus_i => cpu1_periph_dbus_i,
      cpu1_ddr_ibus_o => cpu1_ddr_ibus_o, cpu1_ddr_ibus_i => cpu1_ddr_ibus_i,
      cpu1_ddr_dbus_o => cpu1_ddr_dbus_o, cpu1_ddr_dbus_i => cpu1_ddr_dbus_i,
      cpu1_mem_lock => cpu1_mem_lock,
      debug_i => CPU_DEBUG_NOP, debug_o => debug_o,
      cpu0_data_master_en => cpu0_data_master_en, cpu1_data_master_en => cpu1_data_master_en,
      cpu0_data_master_ack => cpu0_data_master_ack, cpu1_data_master_ack => cpu1_data_master_ack,
      cpu1eni => '0',
      cpu0_event_o => cpu0_event_o, cpu0_event_i => NULL_CPU_EVENT_I,
      cpu1_event_o => cpu1_event_o, cpu1_event_i => NULL_CPU_EVENT_I,
      cpu0_copro_o => cpu0_copro_o, cpu0_copro_i => NULL_COPR_I,
      cpu1_copro_o => cpu1_copro_o, cpu1_copro_i => NULL_COPR_I,
      spi_d_cs_n => spi_d_cs_n, spi_d_sck => spi_d_sck,
      spi_d_mosi => spi_d_mosi, spi_d_miso => spi_d_miso);

  ------------------------------------------------------------------------
  -- Behavioral SPI flash model, wired to cpus_xip.vhd's internal pad-level
  -- pin_* signals via VHDL-2008 external names (see header comment for why
  -- the pad, not the d_* digital side, is the only reachable tap point from
  -- outside the generated `cpus` entity). Drives pin_miso from xip_flash_pkg
  -- (xip_test.img, preloaded at FLASH_IMG_BASE = 0x100000); reads
  -- pin_cs_n/pin_sck/pin_mosi (driven by ice_spi_io's SB_IO output pads).
  ------------------------------------------------------------------------
  flash_model : process
    variable shreg   : std_logic_vector(31 downto 0);
    variable addr    : natural;
    variable tx_byte : std_logic_vector(7 downto 0);
    variable byte_i  : natural;

    -- flash_mem(k): byte at flash offset k. Serves xip_flash_pkg's preloaded
    -- image over [FLASH_IMG_BASE, FLASH_IMG_BASE+FLASH_IMG_LEN); zero
    -- elsewhere (harmless -- the fill engine always fills a full 4 KB page,
    -- and the test program never reads past its own table).
    impure function flash_mem(k : natural) return std_logic_vector is
    begin
      if k >= FLASH_IMG_BASE and (k - FLASH_IMG_BASE) < FLASH_IMG_LEN then
        return FLASH_IMG(k - FLASH_IMG_BASE);
      else
        return x"00";
      end if;
    end function;
  begin
    spi_d_miso <= '0';
    loop
      wait until spi_d_cs_n = '0';

      shreg := (others => '0');
      for i in 0 to 31 loop
        wait until rising_edge(spi_d_sck);
        shreg := shreg(30 downto 0) & spi_d_mosi;
      end loop;
      addr := to_integer(unsigned(shreg(23 downto 0)));

      for i in 0 to 7 loop
        wait until rising_edge(spi_d_sck);
      end loop;

      byte_i := 0;
      outer : loop
        tx_byte := flash_mem(addr + byte_i);
        for b in 7 downto 0 loop
          if spi_d_cs_n = '1' then
            exit outer;
          end if;
          spi_d_miso <= tx_byte(b);
          wait until rising_edge(spi_d_sck) or spi_d_cs_n = '1';
          if spi_d_cs_n = '1' then
            exit outer;
          end if;
        end loop;
        byte_i := byte_i + 1;
      end loop;
    end loop;
  end process;

  ------------------------------------------------------------------------
  -- TEST_RESULT_ADDRESS watcher: taps cpus_xip.vhd's data_master_snoop (the
  -- CPU's pre-decode data-master bus, per Task 4) via an external name, since
  -- 0xBCDE0010 does not decode to any device bus port reachable from outside
  -- cpus (see header comment).
  ------------------------------------------------------------------------
  result_watch : process
    alias ext_data_master_snoop is
      << signal .cpus_xip_tb.dut.data_master_snoop : cpu_data_o_t >>;
  begin
    wait until rising_edge(clk);
    if not done then
      if ext_data_master_snoop.en = '1' and ext_data_master_snoop.wr = '1'
         and ext_data_master_snoop.a = TEST_RESULT_ADDRESS then
        test_result_val  <= ext_data_master_snoop.d;
        test_result_seen <= true;
      end if;
    end if;
  end process;

  ------------------------------------------------------------------------
  -- Top-level sequencing: release reset, run until TEST_RESULT_ADDRESS is
  -- written (PASS iff the written value is 0) or a generous stop-time
  -- elapses (FAIL/timeout). Demand-paging 5+ pages over a bit-banged SPI
  -- Fast-Read (32 bits cmd+addr + 8 dummy + 4096 data bytes, each byte 8 sck
  -- edges) per page fault is slow in simulated time -- tens of ms.
  ------------------------------------------------------------------------
  dbg_prog : process
    variable prevcs : std_logic := '1';
    variable ntx : integer := 0;
  begin
    wait until rising_edge(clk);
    if spi_d_cs_n = '0' and prevcs = '1' then ntx := ntx + 1;
      report "PROG t=" & time'image(now) & " flash_tx#=" & integer'image(ntx)
             severity note; end if;
    prevcs := spi_d_cs_n;
  end process;

  smoke : process
    constant STOP_TIME : time := 60 ms;
  begin
    rst <= '1';
    wait for CLK_PER * 8;
    rst <= '0';

    wait until test_result_seen for STOP_TIME;

    assert test_result_seen
      report "TIMEOUT: no write to TEST_RESULT_ADDRESS (0xBCDE0010) observed within " &
             time'image(STOP_TIME)
      severity failure;

    report "cpus_xip_tb: TEST_RESULT_ADDRESS write observed, value=" &
           to_hstring(test_result_val) severity note;

    assert test_result_val = x"00000000"
      report "Test failed: TEST_RESULT=" & to_hstring(test_result_val) &
             " (expected 0)"
      severity failure;

    report "Test Passed cpus_xip_tb" severity note;
    done <= true;
    wait;
  end process;

end architecture;
