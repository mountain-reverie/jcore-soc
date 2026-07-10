library ieee;
use ieee.std_logic_1164.all;
use work.cpu2j0_pack.all;
use work.data_bus_pack.all;
use work.cpu_core_pack.all;

-- M0 dual-CPU cpus architecture: ECP5 port of targets/cpus_two_fpga.vhd
-- (arch two_cpus_fpga). Substitutes bootram_infer(inferred) for memory_fpga
-- and uses a direct entity/arch binding for ram_2rw(inferred).
architecture two_cpu_m0 of cpus is
  signal cpu0_instr_bus_o : instr_bus_o_t;
  signal cpu0_instr_bus_i : instr_bus_i_t;

  signal cpu0_data_bus_o : data_bus_o_t;
  signal cpu0_data_bus_i : data_bus_i_t;

  signal cpu1_instr_bus_o : instr_bus_o_t;
  signal cpu1_instr_bus_i : instr_bus_i_t;

  signal cpu1_data_bus_o : data_bus_o_t;
  signal cpu1_data_bus_i : data_bus_i_t;

  signal cpu0_ram_o        : cpu_data_o_t;
  signal cpu0_ram_prearb_o : cpu_data_o_t;
  signal cpu0_ram_i : cpu_data_i_t;
  signal cpu0_romd_split_o : cpu_data_o_t;
  signal cpu0_romd_split_i : cpu_data_i_t;
  signal cpu0_romdt_o : cpu_data_o_t;
  signal cpu0_romdt_i : cpu_data_i_t;
  signal cpu0_rominst_o : cpu_instruction_o_t;
  signal cpu0_rominst_i : cpu_instruction_i_t;
  signal cpu1_ram_o : cpu_data_o_t; -- declared to process cpu1en
  signal ram0_arb_o : ram_arb_o_t; -- only three control signals
  signal ram1_arb_o : ram_arb_o_t; -- only three control signals
  signal cpu0ram_a_en : std_logic;
  signal cpu1ram_a_en : std_logic;

  -- A2 (fix/dual-shared-ram-arb-pipeline): 1-cycle-delayed shared_ram ack that
  -- accompanies moving shared_ram from the falling edge (clkn) to the rising
  -- edge (clk). See the shared_ram instance and the ack process below.
  signal cpu0_ram_ack_r : std_logic;
  signal cpu1_ram_ack_r : std_logic;

  signal clkn : std_logic;

  signal cpu0_lock : std_logic;
  signal cpu1_lock : std_logic;

  -- split DEV_SRAM memory bus into ROM and shared RAM buses
  procedure split_local_mem_bus(
    signal master_i : out cpu_data_i_t;
    signal master_o : in  cpu_data_o_t;
    signal rom_i    : in  cpu_data_i_t;
    signal rom_o    : out cpu_data_o_t;
    signal ram_i    : in  cpu_data_i_t;
    signal ram_o    : out cpu_data_o_t) is
  begin
    -- assign request to both ram and rom. en/rd/wr are masked to zero for
    -- unused bus below.
    rom_o <= master_o;
    ram_o <= master_o;

    -- already know a(31 downto 28) = "0000" due to cpu_core decoding
    if master_o.a(27 downto 15) = "0000000000000" then
      -- first 32KB are ROM
      master_i <= rom_i;
      -- prevent ram access
      ram_o.en <= '0';
      ram_o.rd <= '0';
      ram_o.wr <= '0';
    elsif master_o.a(27 downto 11) = "00000000000010000" then
      -- next 2KB are RAM
      master_i <= ram_i;
      -- prevent rom access
      rom_o.en <= '0';
      rom_o.rd <= '0';
      rom_o.wr <= '0';
    else
      -- ignore operations to other memory and return 0
      master_i.ack <= master_o.en;
      master_i.d <= (others => '0');
    end if;
  end;

begin

  cpu0_mem_lock <= cpu0_lock;
  cpu1_mem_lock <= cpu1_lock;

  -- clock memories on negative edge so that memory access are acked before the
  -- end of the cycle.
  -- TODO: Check memory access times are fast enough
  clkn <= not clk;

  -- labels are core0/core1 (not cpu0/cpu1) to avoid clashing with the synopsys
  -- groups "cpu0"/"cpu1" declared in the cpus entity, which ghdl does not skip.
  core0 : cpu_core
    generic map ( COPRO_DECODE => false, CORE_ID => 0 )
    port map (
      clk => clk,
      rst => rst,
      instr_bus_o => cpu0_instr_bus_o,
      instr_bus_i => cpu0_instr_bus_i,
      data_bus_lock => cpu0_lock,
      data_bus_o => cpu0_data_bus_o,
      data_bus_i => cpu0_data_bus_i,
      debug_o => debug_o,
      debug_i => debug_i,
      event_o => cpu0_event_o,
      event_i => cpu0_event_i,
      data_master_en => cpu0_data_master_en,
      data_master_ack => cpu0_data_master_ack,
      copro_o => cpu0_copro_o,
      copro_i => cpu0_copro_i);

  cpu0_periph_dbus_o <= cpu0_data_bus_o(DEV_PERIPH);
  cpu0_data_bus_i(DEV_PERIPH) <= cpu0_periph_dbus_i;

  cpu0_ddr_ibus_o <= cpu0_instr_bus_o(DEV_DDR);
  cpu0_instr_bus_i(DEV_DDR) <= cpu0_ddr_ibus_i;

  cpu0_ddr_dbus_o <= cpu0_data_bus_o(DEV_DDR);
  cpu0_data_bus_i(DEV_DDR) <= cpu0_ddr_dbus_i;

  core1 : cpu_core
    generic map ( COPRO_DECODE => false, CORE_ID => 1 )
    port map (
      clk => clk,
      rst => rst,
      instr_bus_o => cpu1_instr_bus_o,
      instr_bus_i => cpu1_instr_bus_i,
      data_bus_lock => cpu1_lock,
      data_bus_o => cpu1_data_bus_o,
      data_bus_i => cpu1_data_bus_i,
      -- TODO: Add separate debug ports for cpu1
      debug_o => open,
      debug_i => CPU_DEBUG_NOP,
      event_o => cpu1_event_o,
      event_i => cpu1_event_i,
      data_master_en => cpu1_data_master_en,
      data_master_ack => cpu1_data_master_ack,
      copro_o => cpu1_copro_o,
      copro_i => cpu1_copro_i);

  cpu1_periph_dbus_o <= cpu1_data_bus_o(DEV_PERIPH);
  cpu1_data_bus_i(DEV_PERIPH) <= cpu1_periph_dbus_i;

  cpu1_ddr_ibus_o <= cpu1_instr_bus_o(DEV_DDR);
  cpu1_instr_bus_i(DEV_DDR) <= cpu1_ddr_ibus_i;

  cpu1_ddr_dbus_o <= cpu1_data_bus_o(DEV_DDR);
  cpu1_data_bus_i(DEV_DDR) <= cpu1_ddr_dbus_i;

  split_local_mem_bus(
    master_i => cpu0_data_bus_i(DEV_SRAM),
    master_o => cpu0_data_bus_o(DEV_SRAM),
    rom_i    => cpu0_romd_split_i,
    rom_o    => cpu0_romd_split_o,
    ram_i    => cpu0_ram_i,
    ram_o    => cpu0_ram_prearb_o);

  -- cpu1 is not connected to the rom
  cpu1_instr_bus_i(DEV_SRAM) <= loopback_bus(cpu1_instr_bus_o(DEV_SRAM));

  -- cpu0 boot ROM region (ECP5 inferred EBR), fed via the local-mem splitter's
  -- rom side through the bus-delay shims. 14-bit = 16KB, matching one_cpu_m0.
  sram : entity work.bootram_infer(inferred)
    generic map (c_addr_width => 14)
    port map (
      clk    => clk,
      ibus_i => cpu0_rominst_o,
      ibus_o => cpu0_rominst_i,
      db_i   => cpu0_romdt_o,
      db_o   => cpu0_romdt_i);

  bootmem_onewait_data : entity work.data_bus_delay (rtl)
      generic map (INSERT_WRITE_DELAY => INSERT_WRITE_DELAY_BOOT_MEM,
                   INSERT_READ_DELAY  => INSERT_READ_DELAY_BOOT_MEM)
      port map ( clk => clk, rst => rst,
        master_o => cpu0_romd_split_o ,
        master_i => cpu0_romd_split_i ,
        slave_o =>  cpu0_romdt_o ,
        slave_i =>  cpu0_romdt_i );

  bootmem_onewait_inst : entity work.instr_bus_delay (rtl)
      generic map (INSERT_DELAY => INSERT_INST_DELAY_BOOT_MEM)
      port map ( clk => clk, rst => rst,
        master_o => cpu0_instr_bus_o(DEV_SRAM) ,
        master_i => cpu0_instr_bus_i(DEV_SRAM) ,
        slave_o =>  cpu0_rominst_o ,
        slave_i =>  cpu0_rominst_i );

  -- 2KB of shared RAM

  -- A2: shared_ram is now clocked on the rising edge (clk), NOT clkn -- this
  -- removes the historical 0.5-cycle SRAM access critical path (the deep
  -- datapath this_c cone + the lock arbiter no longer have to settle within a
  -- half cycle). Read data lands one cycle later; the ack below is delayed one
  -- cycle to match, so every shared_ram access costs +1 wait state.

  shared_ram : entity work.ram_2rw(inferred)
    generic map (
      SUBWORD_WIDTH => 8,
      SUBWORD_NUM   => 4,
      ADDR_WIDTH    => 9)
    port map (
      rst0 => rst, clk0 => clk,
      en0  => cpu0_ram_o.en, wr0 => cpu0_ram_o.wr, we0 => cpu0_ram_o.we,
      a0   => cpu0_ram_o.a(10 downto 2), dw0 => cpu0_ram_o.d, dr0 => cpu0_ram_i.d,
      rst1 => rst, clk1 => clk,
      en1  => cpu1_ram_o.en, wr1 => cpu1_ram_o.wr, we1 => cpu1_ram_o.we,
      a1   => cpu1_ram_o.a(10 downto 2), dw1 => cpu1_ram_o.d,
      dr1  => cpu1_data_bus_i(DEV_SRAM).d,
      margin0 => '0', margin1 => '0');

  -- cpu0 ram enable (= ram arbitration, lock) processing
  cpu0_ram_o.en  <= cpu0_ram_prearb_o.en and cpu0ram_a_en;
  cpu0_ram_o.wr  <= cpu0_ram_prearb_o.wr and cpu0ram_a_en;
  cpu0_ram_o.we  <= cpu0_ram_prearb_o.we and
                   (cpu0ram_a_en & cpu0ram_a_en &
                    cpu0ram_a_en & cpu0ram_a_en);
  cpu0_ram_o.a   <= cpu0_ram_prearb_o.a;
  cpu0_ram_o.d   <= cpu0_ram_prearb_o.d;

  -- cpu1 ram enable (= not cpu1 halt, ram arbitration, lock)) processing
  cpu1_ram_o.en  <= cpu1_data_bus_o(DEV_SRAM).en and cpu1ram_a_en;
  cpu1_ram_o.wr  <= cpu1_data_bus_o(DEV_SRAM).wr and cpu1ram_a_en;
  cpu1_ram_o.we  <= cpu1_data_bus_o(DEV_SRAM).we and
                   (cpu1ram_a_en & cpu1ram_a_en &
                    cpu1ram_a_en & cpu1ram_a_en);
  cpu1_ram_o.a   <= cpu1_data_bus_o(DEV_SRAM).a;
  cpu1_ram_o.d   <= cpu1_data_bus_o(DEV_SRAM).d;

  -- ack for shared_ram: the RAM is now rising-edge clocked (clk0/clk1 => clk),
  -- so read data lands one cycle after the request. Assert ack for exactly the
  -- cycle the data is valid. "en and not ack_r" is a one-cycle pulse per access
  -- (1 wait state) that self-clears while en is still held, so back-to-back
  -- accesses each ack exactly once. A blocked access (a_en=0 -> en=0) never
  -- acks, so the losing core stalls until the lock releases -- same as before.
  arb_ack : process(clk, rst)
  begin
    if rst = '1' then
      cpu0_ram_ack_r <= '0';
      cpu1_ram_ack_r <= '0';
    elsif clk'event and clk = '1' then
      cpu0_ram_ack_r <= cpu0_ram_o.en and not cpu0_ram_ack_r;
      cpu1_ram_ack_r <= cpu1_ram_o.en and not cpu1_ram_ack_r;
    end if;
  end process;
  cpu0_ram_i.ack                <= cpu0_ram_ack_r;
  cpu1_data_bus_i(DEV_SRAM).ack <= cpu1_ram_ack_r;

  cpumreg : entity work.cpumreg
    port map (
      clk => clk,
      rst => rst,
      db0_i => cpu0_data_bus_o(DEV_CPU),
      db1_i => cpu1_data_bus_o(DEV_CPU),
      ram0_arb_o => ram0_arb_o,
      ram1_arb_o => ram1_arb_o,
      db0_o => cpu0_data_bus_i(DEV_CPU),
      db1_o => cpu1_data_bus_i(DEV_CPU),
      cpu0ram_a_en => cpu0ram_a_en,
      cpu1ram_a_en => cpu1ram_a_en,
      cpu1en_sbu => cpu1eni);

   ram0_arb_o.en   <= cpu0_ram_prearb_o.en;
   ram0_arb_o.wr   <= cpu0_ram_prearb_o.wr;
   ram0_arb_o.lock <= cpu0_lock;
   ram1_arb_o.en   <= cpu1_data_bus_o(DEV_SRAM).en;
   ram1_arb_o.wr   <= cpu1_data_bus_o(DEV_SRAM).wr;
   ram1_arb_o.lock <= cpu1_lock;

end architecture;
