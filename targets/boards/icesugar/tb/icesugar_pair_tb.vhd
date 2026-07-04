library ieee;
use ieee.std_logic_1164.all;
use ieee.numeric_std.all;

-- Two cross-connected iCESugar J1 SoCs (pad_ring instances "a" and "b"), each's
-- real eth_tx_phy MDI driving the peer's real eth_rx_phy MDI input -- no
-- Manchester stimulus process, no SPRAM memtest. Both run the ETH_PAIR_TEST
-- boot image (boot_image_pairtest_pkg.vhd, swapped in for boot_image_pkg.vhd by
-- pair_sim.sh), which sends a gratuitous ARP request for OUR_IP right after
-- boot. Since both instances share the same OUR_IP/OUR_MAC (banner.c), each
-- answers the other's request with an ARP reply. b's oscillator is gated on
-- ~2 ms after a's, so a is already in its eth_recv() poll loop when b sends its
-- gratuitous request: a receives that request through its REAL eth_rx PHY and
-- answers it. We decode a's mdi0 and assert the reply matches the expected
-- ARP-reply frame -- validating the RX path end-to-end (PHY differential decode
-- -> framer -> eth_recv -> eth_handle -> eth_send -> PHY encode) in a few ms of
-- sim time, without the ~139 ms SPRAM memtest.
entity icesugar_pair_tb is end entity;

architecture sim of icesugar_pair_tb is
  constant CLK_PER : time := 1 sec / 12_000_000;   -- 12 MHz oscillator
  constant HALF_BIT : time := 50 ns;   -- Manchester half-bit slot, see icesugar_top_tb

  signal clk_a, clk_b : std_logic := '0';
  -- Two independent boards never boot in lockstep: gate b's oscillator on for a
  -- couple of ms after a's, so the pair does not transmit/receive in perfect
  -- phase (which would collide every frame). This de-correlates the two so that
  -- exactly one node (a) is quiet and polling when the other (b) sends its
  -- gratuitous ARP request -- the RX-triggered reply we validate.
  signal b_clk_en : boolean := false;
  signal ser_rx_a, ser_rx_b : std_logic := '1';
  signal ser_tx_a, ser_tx_b : std_logic;
  signal ledr_n_a, ledg_n_a, ledb_n_a : std_logic;
  signal ledr_n_b, ledg_n_b, ledb_n_b : std_logic;
  signal mdi0_p_a, mdi0_n_a : std_logic;
  signal mdi0_p_b, mdi0_n_b : std_logic;

  signal done   : boolean := false;
  signal arp_ok : boolean := false;

  -- Expected ARP reply on a's mdi0 (a replying to b's gratuitous ARP request):
  -- both instances share OUR_MAC/OUR_IP (banner.c), so the reply's eth dest /
  -- ARP target HA+PA equal the sender's own identity. Same convention as
  -- icesugar_top_tb's REP_FRAME; the FCS is the IEEE 802.3 CRC-32 of this exact
  -- 42-byte reply body (0x2462A207), appended little-endian.
  constant NREP : integer := 54;
  type frame54_t is array (0 to NREP - 1) of std_logic_vector(7 downto 0);
  constant REP_FRAME : frame54_t := (
    0 => x"55", 1 => x"55", 2 => x"55", 3 => x"55",
    4 => x"55", 5 => x"55", 6 => x"55",             -- preamble x7
    7 => x"D5",                                      -- SFD
    8 => x"02", 9 => x"00", 10 => x"00",
    11 => x"00", 12 => x"00", 13 => x"01",           -- eth dest: OUR_MAC (peer asked)
    14 => x"02", 15 => x"00", 16 => x"00",
    17 => x"00", 18 => x"00", 19 => x"01",           -- eth src: OUR_MAC (we answer)
    20 => x"08", 21 => x"06",                        -- ethertype: ARP
    22 => x"00", 23 => x"01",                        -- htype = Ethernet
    24 => x"08", 25 => x"00",                        -- ptype = IPv4
    26 => x"06",                                      -- hlen
    27 => x"04",                                      -- plen
    28 => x"00", 29 => x"02",                        -- opcode = reply
    30 => x"02", 31 => x"00", 32 => x"00",
    33 => x"00", 34 => x"00", 35 => x"01",           -- sender HA: OUR_MAC
    36 => x"C0", 37 => x"A8", 38 => x"01", 39 => x"0A", -- sender PA: OUR_IP
    40 => x"02", 41 => x"00", 42 => x"00",
    43 => x"00", 44 => x"00", 45 => x"01",           -- target HA: OUR_MAC (peer, echoed)
    46 => x"C0", 47 => x"A8", 48 => x"01", 49 => x"0A", -- target PA: OUR_IP (echoed)
    50 => x"07", 51 => x"A2", 52 => x"62", 53 => x"24"); -- FCS (LE), CRC32=0x2462A207
begin
  a : entity work.pad_ring(impl)
    port map (pin_clk => clk_a, pin_ser_rx => ser_rx_a, pin_ser_tx => ser_tx_a,
              pin_ledr_n => ledr_n_a, pin_ledg_n => ledg_n_a, pin_ledb_n => ledb_n_a,
              pin_mdi0_p => mdi0_p_a, pin_mdi0_n => mdi0_n_a, pin_mdi1_p => mdi0_p_b);

  b : entity work.pad_ring(impl)
    port map (pin_clk => clk_b, pin_ser_rx => ser_rx_b, pin_ser_tx => ser_tx_b,
              pin_ledr_n => ledr_n_b, pin_ledg_n => ledg_n_b, pin_ledb_n => ledb_n_b,
              pin_mdi0_p => mdi0_p_b, pin_mdi0_n => mdi0_n_b, pin_mdi1_p => mdi0_p_a);

  clk_a <= not clk_a after CLK_PER/2 when not done else '0';
  clk_b <= not clk_b after CLK_PER/2 when (b_clk_en and not done) else '0';
  b_clk_en <= true after 2 ms;

  -- Manchester decoder on a's mdi0. a boots first (b's oscillator is gated on
  -- 2 ms later), so a is already in its eth_recv poll loop when b -- once
  -- booted -- sends its gratuitous ARP request. a receives that request through
  -- its real eth_rx PHY and answers with an ARP reply, decoded here. Same
  -- skip-NLP-pulse convention as icesugar_top_tb.
  eth_decode : process
    variable t0      : time;
    variable got_rep : frame54_t;
    variable bitv    : std_logic;
    variable bad     : boolean;
  begin
    -- a's mdi0 carries a's own gratuitous ARP REQUEST (opcode 1, does not match
    -- REP_FRAME) and then a's ARP REPLY (opcode 2) to b's request -- the
    -- RX-triggered frame we validate (the 28-byte ETH TX test frame is omitted
    -- in the pair build, see banner.c). Decode frames repeatedly, comparing
    -- each against REP_FRAME, until the reply is seen (bounded by the tb
    -- watchdog). A decode that runs into an illegal "00/00" differential
    -- mid-frame (a short frame ending, or an NLP idle pulse) just abandons that
    -- attempt and resumes scanning.
    decode_loop : loop
      loop
        wait until mdi0_p_a = '1' or mdi0_n_a = '1';
        t0 := now;
        wait for HALF_BIT + HALF_BIT/2;
        if mdi0_p_a = '0' and mdi0_n_a = '0' then
          next;   -- spurious NLP pulse; keep scanning
        end if;
        exit;
      end loop;

      bad := false;
      byte_loop : for i in 0 to NREP - 1 loop
        for j in 0 to 7 loop
          wait for (t0 + (real(16*i + 2*j + 1) + 0.5) * HALF_BIT) - now;
          bitv := mdi0_p_a;
          if mdi0_p_a = mdi0_n_a then
            bad := true;
            exit byte_loop;   -- not a real frame bit; abandon, resync above
          end if;
          got_rep(i)(j) := bitv;
        end loop;
      end loop;

      if not bad then
        report "icesugar_pair_tb: decoded frame at " & time'image(t0)
          & " opcode=" & integer'image(to_integer(unsigned(got_rep(29))))
          severity note;
      end if;

      if not bad and got_rep = REP_FRAME then
        report "icesugar_pair_tb: PAIR ARP OK (decoded+CRC-verified 54-byte ARP reply, a answering b)"
          severity note;
        arp_ok <= true;
        exit decode_loop;
      end if;
    end loop;
    wait;
  end process;

  pass_gate : process begin
    if not arp_ok then wait until arp_ok; end if;
    done <= true;
    wait;
  end process;

  watchdog : process begin
    wait for 15 ms;
    assert done report "TIMEOUT: PAIR ARP OK not seen" severity failure;
    wait;
  end process;
end architecture;
