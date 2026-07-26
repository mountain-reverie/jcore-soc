library ieee;
  use ieee.std_logic_1164.all;
  use ieee.numeric_std.all;
  use work.cpu2j0_pack.all;
  use work.ddrc_cnt_pack.all;
  use work.data_bus_pack.all;

package cache_pack is

  constant cache_line_width_bits : natural := 5; -- 32 byte lines

  -- * * * * * start of configurable i-cache capacity ----
  -- standard i-cache capacity ----
  constant cache_index_bits : natural := 6; -- 8k byte cache
  -- + other configuration
  -- + constant CACHE_INDEX_BITS        : natural := 7;  -- 4k byte cache
  -- + constant CACHE_INDEX_BITS        : natural := 9;  -- 16k byte cache
  -- * * * * * end of configurable i-cache capacity ----

  constant cache_valid_bits     : natural := 3;  -- 8 bit wide valid reg file
  constant cache_mem_width_bits : natural := 2;  -- 4  byte to ddr
  constant cache_region_width   : natural := 28; -- 256M byte / 2G bit
  constant cache_i_width_bits   : natural := 1;  -- 16 bit instructions
  constant cache_d_width_bits   : natural := 2;  -- 32 bit data

  constant cache_tag_width    : natural := CACHE_REGION_WIDTH - CACHE_LINE_WIDTH_BITS - CACHE_INDEX_BITS;
  constant cache_pa_tag_width : natural := CACHE_TAG_WIDTH; -- PA tag = CACHE_REGION_WIDTH - CACHE_LINE_WIDTH_BITS - CACHE_INDEX_BITS bits (15 b @ 8KB; widens as the cache shrinks)

  type mmu_cache_i_t is record
    pa_tag : std_logic_vector(CACHE_PA_TAG_WIDTH - 1 downto 0);
    at     : std_logic;
    c      : std_logic; -- PTE C-bit (cacheable); meaningful only when at='1'
  end record mmu_cache_i_t;

  constant mmu_cache_i_reset    : mmu_cache_i_t := (pa_tag => (others => '0'), at => '0', c => '0');
  constant cache_index_msb      : natural       := CACHE_LINE_WIDTH_BITS + CACHE_INDEX_BITS - 1;
  constant cache_line_width     : natural       := 2 ** CACHE_LINE_WIDTH_BITS;
  constant cache_line_mem_words : natural       := 2 ** (CACHE_LINE_WIDTH_BITS - CACHE_MEM_WIDTH_BITS);
  constant cache_lines          : natural       := 2 ** CACHE_INDEX_BITS;
  constant cache_valid_width    : natural       := 2 ** CACHE_VALID_BITS;
  constant cache_mem_width      : natural       := 2 ** (CACHE_MEM_WIDTH_BITS + 3);
  constant cache_mem_words      : natural       := 2 ** (CACHE_INDEX_BITS + CACHE_LINE_WIDTH_BITS - CACHE_MEM_WIDTH_BITS);
  constant cache_i_width        : natural       := 2 ** (CACHE_I_WIDTH_BITS + 3);
  constant cache_d_width        : natural       := 2 ** (CACHE_D_WIDTH_BITS + 3);

  constant cache_dcmd_readmiss    : std_logic_vector(2 downto 0) := b"000";
  constant cache_dcmd_negate_lo   : std_logic_vector(2 downto 0) := b"001";
  constant cache_dcmd_readsgl_nm  : std_logic_vector(2 downto 0) := b"010";
  constant cache_dcmd_readsgl_lo  : std_logic_vector(2 downto 0) := b"011";
  constant cache_dcmd_writemiss   : std_logic_vector(2 downto 0) := b"100";
  constant cache_dcmd_writesgl_fa : std_logic_vector(2 downto 0) := b"110";
  constant cache_dcmd_writesgl_sl : std_logic_vector(2 downto 0) := b"111";

  function to_cache_tag (
    a : std_logic_vector
  ) return std_logic_vector;

  function to_cache_valid (
    a : std_logic_vector
  ) return integer;

  function to_cache_vbit (
    a : std_logic_vector
  ) return integer;

  function to_cache_index (
    a : std_logic_vector
  ) return integer;

  function to_cache_offset (
    a : std_logic_vector
  ) return integer;

  function to_cache_addr (
    a : std_logic_vector
  ) return integer;

  function to_cache_idata (
    a,
    d : std_logic_vector
  ) return std_logic_vector;

  function itov (
    x,
    n : integer
  ) return std_logic_vector;

  function vtoi (
    x : std_logic_vector
  ) return integer;

  -- SH P-region cacheability (region-only; PTE C-bit deferred).
  -- Cached: P0 0x0-0x7, P1 0x8-0x9, P3 0xC-0xD. Uncached: P2 0xA-0xB, P4 0xE-0xF.

  function is_cacheable (
    a : std_logic_vector(31 downto 0)
  ) return boolean;

  -- Combined cacheability: when address-translated (at='1') the PTE C-bit is
  -- authoritative (a translated access is always in a cacheable region); when
  -- untranslated (at='0', MMU off or P1/P2/P4) fall back to region decode.

  function is_cacheable_mmu (
    a : std_logic_vector(31 downto 0);
    at : std_logic;
    c : std_logic
  ) return boolean;

  type mem_i_t is record
    d     : std_logic_vector(CACHE_MEM_WIDTH - 1 downto 0);
    ack   : std_logic;
    ack_r : std_logic;
  end record mem_i_t;

  type mem_o_t is record
    a        : std_logic_vector(CACHE_REGION_WIDTH - 1 downto 0);
    av       : std_logic;
    lock     : std_logic;
    ddrburst : std_logic;
    en       : std_logic;
    d        : std_logic_vector(CACHE_MEM_WIDTH - 1 downto 0);
    wr       : std_logic;
    we       : std_logic_vector(CACHE_MEM_WIDTH / 8 - 1 downto 0);
  end record mem_o_t;

  constant mem_o_reset : mem_o_t :=
  (
    (others => '0'),
    '0',
    '0',
    '0',
    '0',
    (others => '0'),
    '0',
    (others => '0')
  );

  type icache_state_t is (idle, miss1, miss2, miss3, off1, off2);

  type icache_o_t is record
    d   : std_logic_vector(CACHE_I_WIDTH - 1 downto 0);
    ack : std_logic;
  end record icache_o_t;

  constant icache_o_reset : icache_o_t := ((others => '0'), '0');

  type icache_ram_i_t is record
    a0  : std_logic_vector(CACHE_INDEX_BITS + CACHE_LINE_WIDTH_BITS - CACHE_MEM_WIDTH_BITS - 1 downto 0);
    en0 : std_logic;
    a1  : std_logic_vector(CACHE_INDEX_BITS + CACHE_LINE_WIDTH_BITS - CACHE_MEM_WIDTH_BITS - 1 downto 0);
    en1 : std_logic;
    wr1 : std_logic;
    d1  : std_logic_vector(CACHE_MEM_WIDTH - 1   downto 0);
    ta  : std_logic_vector(CACHE_INDEX_BITS - 1 downto 0);
    ten : std_logic;
    twr : std_logic;
    tag : std_logic_vector(CACHE_PA_TAG_WIDTH - 1   downto 0);
  end record icache_ram_i_t;

  type icache_ramccl_i_t is record
    a0  : std_logic_vector(CACHE_INDEX_BITS + CACHE_LINE_WIDTH_BITS - CACHE_MEM_WIDTH_BITS - 1 downto 0);
    en0 : std_logic;
    ta  : std_logic_vector(CACHE_INDEX_BITS - 1 downto 0);
    ten : std_logic;
    twr : std_logic;
    tag : std_logic_vector(CACHE_PA_TAG_WIDTH - 1   downto 0);
  end record icache_ramccl_i_t;

  type icache_rammcl_i_t is record
    a1  : std_logic_vector(CACHE_INDEX_BITS + CACHE_LINE_WIDTH_BITS - CACHE_MEM_WIDTH_BITS - 1 downto 0);
    en1 : std_logic;
    wr1 : std_logic;
    d1  : std_logic_vector(CACHE_MEM_WIDTH - 1   downto 0);
  end record icache_rammcl_i_t;

  type icache_ram_o_t is record
    d0  : std_logic_vector(CACHE_MEM_WIDTH - 1     downto 0);
    tag : std_logic_vector(CACHE_PA_TAG_WIDTH - 1  downto 0);
  end record icache_ram_o_t;

  type icache_i_t is record
    a   : std_logic_vector(CACHE_REGION_WIDTH - 1 downto 0);
    en  : std_logic;
    mmu : mmu_cache_i_t;
  end record icache_i_t;

  type icccr_i_t is record
    ic_onm : std_logic;
    ic_inv : std_logic;
  end record icccr_i_t;

  type cache_v_ff_t is array(0 to CACHE_LINES - 1) of std_logic;

  type icacheccl_reg_t is record
    state      : icache_state_t;
    ma0        : std_logic_vector(27 downto 0);
    ma0_at     : std_logic;
    ma0_pa_tag : std_logic_vector(CACHE_PA_TAG_WIDTH - 1 downto 0);
    a_prev     : icache_i_t;
    a_prev_v   : std_logic;
    c_hitstate : std_logic;
    pref_inc   : std_logic;
    aen_del1   : std_logic;
    dw         : std_logic_vector(31 downto 0);
    ry_twr     : std_logic;
    ry_en0     : std_logic;
    ffv        : cache_v_ff_t;
    rfillv     : std_logic;
    mvbank     : std_logic;
    cd0        : std_logic_vector(31 downto 0);
    cd1        : std_logic_vector(31 downto 0);
    ic_onm     : std_logic;
    ic_onmprep : std_logic;
    ic_inv     : std_logic;
  end record icacheccl_reg_t;

  constant icachecclk_reg_reset : icacheccl_reg_t :=
  (
    IDLE,                                      -- state
    (others  => '0'),                          -- ma0
    '0',                                       -- ma0_at
    (others  => '0'),                          -- ma0_pa_tag
    ((others => '0'), '0', MMU_CACHE_I_RESET), -- a_prev
    '0',                                       -- a_prev_v
    '0',                                       -- c_hitkp
    '0',                                       -- pref_inc
    '0',                                       -- aen_del1
    (others  => '0'),                          -- dw
    '0',                                       -- ry_twr
    '0',                                       -- ry_en0
    (others  => '0'),                          -- ffv
    '0',                                       -- rfillv
    '0',                                       -- mvbank
    (others  => '0'),                          -- cd0
    (others  => '0'),                          -- cd1
    '0',                                       -- ic_onm
    '0',                                       -- ic_onmprep
    '0'                                        -- ic_inv
  );

  type icachemcl_reg_t is record
    ma0         : std_logic_vector(27 downto 0);
    ma0_1in     : std_logic;
    reqw        : std_logic_vector(2 downto 0);
    men         : std_logic;
    mav         : std_logic;
    wv          : std_logic_vector(15 downto 0);
    d           : std_logic_vector(15 downto 0);
    fillv_del1  : std_logic;
    ry_a1fillc  : std_logic_vector(2 downto 0);
    ry_en1      : std_logic;
    rfillv      : std_logic;
    rfillv_del1 : std_logic;
    rfillv_del2 : std_logic;
    mvbank      : std_logic;
    cd          : std_logic_vector(31 downto 0);
    ic_onm      : std_logic;
  end record icachemcl_reg_t;

  constant icachemclk_reg_reset : icachemcl_reg_t :=
  (
    (others => '0'), -- ma0
    '0',             -- ma0_1del1
    (others => '0'), -- reqw
    '0',             -- men
    '0',             -- mav
    (others => '0'), -- wv
    (others => '0'), -- d
    '0',             -- fillv_del1
    (others => '0'), -- ry_a1fillc
    '0',             -- ry_en1
    '0',             -- rfillv
    '0',             -- rfillv_del1
    '0',             -- rfillv_del2
    '0',             -- mvbank
    (others => '0'), -- cd
    '0'              -- ic_onm
  );

  type ctom_t is record
    fillv : std_logic;
    filla : std_logic_vector(28 downto 0);
  end record ctom_t;

  type mtoc_t is record
    rfillv         : std_logic;
    rfillv_advance : std_logic;
    rfilld         : std_logic_vector(16 downto 0);
    v              : std_logic_vector(15 downto 0);
    cd             : std_logic_vector(31 downto 0);
  end record mtoc_t;

  type ctom_dc_t is record
    b0en  : std_logic;
    b0d   : std_logic_vector(64 downto 0);
    b2en  : std_logic;
    b30en : std_logic;
    b31en : std_logic;
  end record ctom_dc_t;

  type mtoc_dc_t is record
    b0enr        : std_logic;
    b0enr_mcdata : std_logic;
    b0d_unc      : std_logic_vector(31 downto 0);
    b2enr        : std_logic;
    b2d_cfil     : std_logic_vector(31 downto 0);
    b30enr       : std_logic;
    b31enr       : std_logic;
  end record mtoc_dc_t;

  type fsync_cross_t is (value, metastable, stable);

  type fsync_enable_t is array (fsync_cross_t range VALUE to STABLE)
     of std_logic;

  -- icache sync ff array

  type fsync_data_t is array (fsync_cross_t range VALUE to STABLE)
     of std_logic_vector(28 downto 0);

  type fsync_2_data_t is array (fsync_cross_t range METASTABLE to STABLE)
     of std_logic_vector(15 downto 0);

  -- dcache sync ff array

  type fsync_data_dcb_t is array (fsync_cross_t range METASTABLE to STABLE)
     of std_logic_vector(64 downto 0);

  type fsync_data_dcs_t is array (fsync_cross_t range METASTABLE to STABLE)
     of std_logic_vector(31 downto 0);

  type cache_fsync_reg_t is record
    en : fsync_enable_t;
    d  : fsync_data_t;
  end record cache_fsync_reg_t;

  type cache_fsync_2_reg_t is record
    dummy : std_logic;
    v     : fsync_2_data_t;
  end record cache_fsync_2_reg_t;

  -- -----------------------------------------------------------------------------
  -- dcache fsync spec
  -- -----------------------------------------------------------------------------
  --  clk-                 value  meta   stable
  --  stage                |      stable |
  -- -----------------------------------------------------------------------------
  -- b0 :            cclk  65  -> 32  -> 32     part 1
  --                 mclk  32  -> 65  -> 65     part 2
  -- -----------------------------------------------------------------------------
  -- b2 :            cclk   0  -> 32  -> 32     part 3
  --                 mclk  32  ->  0  ->  0     part 4
  -- -----------------------------------------------------------------------------
  -- sum(b30,b31) :  cclk   0  ->  0  ->  0
  --                 mclk   0  ->  0  ->  0
  -- -----------------------------------------------------------------------------

  type dcache_fsync_cc_reg_t is record
    en0  : fsync_enable_t;
    d0v  : std_logic_vector(64 downto 0); -- part1 value
    d0s  : fsync_data_dcs_t;              -- part1 MS, S
    en2  : fsync_enable_t;
    d2s  : fsync_data_dcs_t;              -- part3 MS, S
    en30 : fsync_enable_t;
    en31 : fsync_enable_t;
  end record dcache_fsync_cc_reg_t;

  type dcache_fsync_mc_reg_t is record
    en0  : fsync_enable_t;
    d0v  : std_logic_vector(31 downto 0); -- part2 value
    d0b  : fsync_data_dcb_t;              -- part2 MS, S
    en2  : fsync_enable_t;
    d2v  : std_logic_vector(31 downto 0); -- part 4 value
    en30 : fsync_enable_t;
    en31 : fsync_enable_t;
  end record dcache_fsync_mc_reg_t;

  constant cache_fsync_reg_reset   : cache_fsync_reg_t   :=
  (
    (others => '0'),
    (others => (others => '0'))
  );
  constant cache_fsync_2_reg_reset : cache_fsync_2_reg_t :=
  (
    '0',
    (others => (others => '0'))
  );

  constant dcache_fsync_cc_reg_reset : dcache_fsync_cc_reg_t :=
  (
    (others => '0'),
    (others => '0'),
    (others => (others => '0')),
    (others => '0'),
    (others => (others => '0')),
    (others => '0'),
    (others => '0')
  );
  constant dcache_fsync_mc_reg_reset : dcache_fsync_mc_reg_t :=
  (
    (others => '0'),
    (others => '0'),
    (others => (others => '0')),
    (others => '0'),
    (others => '0'),
    (others => '0'),
    (others => '0')
  );

  type dcache_state_t is (
    idle, rmiss1, rmiss2, rmiss3, wmiss1, wmiss2,
    sb1, sb2, off1, off2, rlock1, rlock2, neglck, wunca1, wunca2
  );

  type dcachemcl_state_t is (idle, rfill, wfill, wtha, wthi, rsg, wsg);

  type dcachemwi_state_t is (idle, gap1, gap2s, gap2r, gap3s, gap3r);

  type dcache_ram_i_t is record
    a0   : std_logic_vector(CACHE_INDEX_BITS + CACHE_LINE_WIDTH_BITS - CACHE_MEM_WIDTH_BITS - 1 downto 0);
    en0  : std_logic;
    wr0  : std_logic;
    we0  : std_logic_vector(CACHE_D_WIDTH / 8 - 1   downto 0);
    d0   : std_logic_vector(CACHE_MEM_WIDTH - 1   downto 0);
    a1   : std_logic_vector(CACHE_INDEX_BITS + CACHE_LINE_WIDTH_BITS - CACHE_MEM_WIDTH_BITS - 1 downto 0);
    en1  : std_logic;
    wr1  : std_logic;
    we1  : std_logic_vector(CACHE_MEM_WIDTH / 8 - 1   downto 0);
    d1   : std_logic_vector(CACHE_MEM_WIDTH - 1   downto 0);
    ta0  : std_logic_vector(CACHE_INDEX_BITS - 1 downto 0);
    ten0 : std_logic;
    twr0 : std_logic;
    tag0 : std_logic_vector(CACHE_PA_TAG_WIDTH - 1   downto 0);
    ta1  : std_logic_vector(CACHE_INDEX_BITS - 1 downto 0);
  end record dcache_ram_i_t;

  type dcache_ramccl_i_t is record
    a0   : std_logic_vector(CACHE_INDEX_BITS + CACHE_LINE_WIDTH_BITS - CACHE_MEM_WIDTH_BITS - 1 downto 0);
    en0  : std_logic;
    wr0  : std_logic;
    we0  : std_logic_vector(CACHE_MEM_WIDTH / 8 - 1   downto 0);
    d0   : std_logic_vector(CACHE_MEM_WIDTH - 1   downto 0);
    ta0  : std_logic_vector(CACHE_INDEX_BITS - 1 downto 0);
    ten0 : std_logic;
    twr0 : std_logic;
    tag0 : std_logic_vector(CACHE_PA_TAG_WIDTH - 1   downto 0);
    ta1  : std_logic_vector(CACHE_INDEX_BITS - 1 downto 0);
  end record dcache_ramccl_i_t;

  type dcache_rammcl_i_t is record
    a1  : std_logic_vector(CACHE_INDEX_BITS + CACHE_LINE_WIDTH_BITS - CACHE_MEM_WIDTH_BITS - 1 downto 0);
    en1 : std_logic;
    wr1 : std_logic;
    we1 : std_logic_vector(CACHE_MEM_WIDTH / 8 - 1   downto 0);
    d1  : std_logic_vector(CACHE_MEM_WIDTH - 1   downto 0);
  end record dcache_rammcl_i_t;

  type dcache_ram_o_t is record
    d0   : std_logic_vector(CACHE_MEM_WIDTH - 1     downto 0);
    tag0 : std_logic_vector(CACHE_PA_TAG_WIDTH - 1  downto 0);
    tag1 : std_logic_vector(CACHE_PA_TAG_WIDTH - 1  downto 0);
  end record dcache_ram_o_t;

  type dcache_snoop_io_t is record
    al : std_logic_vector(CACHE_REGION_WIDTH - CACHE_LINE_WIDTH_BITS - 1 downto 0);
    en : std_logic;
  end record dcache_snoop_io_t;

  constant null_snoop_io : dcache_snoop_io_t := (al => (others => '0'), en => '0');

  type dcacheccl_reg_t is record
    state         : dcache_state_t;
    state_del1    : dcache_state_t;
    ma0           : std_logic_vector(27 downto 0);
    ma0_at        : std_logic;
    ma0_pa_tag    : std_logic_vector(CACHE_PA_TAG_WIDTH - 1 downto 0);
    a_prev        : cpu_data_o_t;
    a_prev_v      : std_logic;
    a_prev_mmu    : mmu_cache_i_t;
    sa_al         : std_logic_vector(CACHE_REGION_WIDTH - CACHE_LINE_WIDTH_BITS - 1 downto 0);
    sa_en_state   : std_logic;
    saout_al1     : std_logic_vector(CACHE_REGION_WIDTH - CACHE_LINE_WIDTH_BITS - 1 downto 0);
    saout_al2a    : std_logic_vector(CACHE_REGION_WIDTH - CACHE_LINE_WIDTH_BITS - 1 downto 0);
    saout_al2b    : std_logic_vector(CACHE_REGION_WIDTH - CACHE_LINE_WIDTH_BITS - 1 downto 0);
    sabank_send   : std_logic;
    sabank_rcv    : std_logic;
    ta0           : std_logic_vector(CACHE_INDEX_MSB - 5 downto 0);
    c_hitstate    : std_logic;
    memlock_state : std_logic;
    pref_inc      : std_logic;
    aen_del1      : std_logic;
    aenrd_del1    : std_logic;
    dw            : std_logic_vector(31 downto 0);
    ry_twr        : std_logic;
    ry_en0        : std_logic;
    ffv           : cache_v_ff_t;
    rfillv        : std_logic;
    dc_onm        : std_logic;
    dc_inv        : std_logic;
    b2en_r        : std_logic;
    b3en_r        : std_logic_vector(1 downto 0);
  end record dcacheccl_reg_t;

  constant dcachecclk_reg_reset : dcacheccl_reg_t :=
  (
    IDLE,                                    -- state
    IDLE,                                    -- state_del1
    (others       => '0'),                   -- ma0
    '0',                                     -- ma0_at
    (others       => '0'),                   -- ma0_pa_tag
    ('0', (others => '0'), '0', '0',
      (others     => '0'), (others => '0')), -- a_prev
    '0',                                     -- a_prev_v
    MMU_CACHE_I_RESET,                       -- a_prev_mmu
    (others       => '0'),                   -- sa_al
    '0',                                     -- sa_en_state
    (others       => '0'),                   -- saout_al1
    (others       => '0'),                   -- saout_al2a
    (others       => '0'),                   -- saout_al2b
    '0',                                     -- sabank_send
    '0',                                     -- sabank_rcv
    (others       => '0'),                   -- ta0
    '0',                                     -- c_hitstate
    '0',                                     -- memlock_state
    '0',                                     -- pref_inc
    '0',                                     -- aen_del1
    '0',                                     -- aenrd_del1
    (others       => '0'),                   -- dw
    '0',                                     -- ry_twr
    '0',                                     -- ry_en0
    (others       => '0'),                   -- ffv
    '0',                                     -- rfillv
    '0',                                     -- dc_onm
    '0',                                     -- dc_in
    '0',                                     -- b2en_r
    "00"                                     -- b3en_r
  );

  type dcachemcl_reg_t is record
    statemcl    : dcachemcl_state_t;
    statemwi    : dcachemwi_state_t;
    ma0         : std_logic_vector(27 downto 0);
    reqw        : std_logic_vector(2 downto 0);
    men         : std_logic;
    mav         : std_logic;
    mwr         : std_logic;
    mlock       : std_logic;
    sbwe        : std_logic_vector(3 downto 0);
    sbbufin     : std_logic;
    sbbufin_2   : std_logic; -- to improve 200MHz my.en delay (for fpga syn)
    sbdata      : std_logic_vector(31 downto 0);
    sbblknxst   : std_logic;
    b0d_unc     : std_logic_vector(31 downto 0);
    fillv_del1  : std_logic;
    ry_a1fillc  : std_logic_vector(2 downto 0);
    ry_en1      : std_logic;
    ry_we1      : std_logic_vector(3 downto 0);
    rfillv_pre1 : std_logic;
    rfillv      : std_logic;
    rfillv_del1 : std_logic;
    cd          : std_logic_vector(31 downto 0);
    b2en        : std_logic;
    b3enr_pls   : std_logic;
    b3enr_dir   : std_logic;
    b30en       : std_logic;
    cmd         : std_logic_vector(2 downto 0);
    ddrburst    : std_logic;
  end record dcachemcl_reg_t;

  constant dcachemclk_reg_reset : dcachemcl_reg_t :=
  (
    IDLE,            -- statemcl
    IDLE,            -- statemwi
    (others => '0'), -- ma0
    (others => '0'), -- reqw
    '0',             -- men
    '0',             -- mav
    '0',             -- mwr
    '0',             -- mlock
    (others => '0'), -- sbwe
    '0',             -- sbbufin
    '0',             -- sbbufin_2
    (others => '0'), -- sbdata
    '0',             -- sbblknxst
    (others => '0'), -- b0d_unc
    '0',             -- fillv_del1
    (others => '0'), -- ry_a1fillc
    '0',             -- ry_en1
    (others => '0'), -- ry_we1
    '0',             -- rfillv_pre1
    '0',             -- rfillv
    '0',             -- rfillv_del1
    (others => '0'), -- cd
    '0',             -- b2en
    '0',             -- b3enr_pls
    '0',             -- b3enr_dir
    '0',             -- b30en
    (others => '0'), -- cmd
    '0'              -- ddrburst
  );

  type tracpu_data_o_t is record
    en : std_logic;
    wr : std_logic;
    a  : std_logic_vector(31 downto 0);
    d  : std_logic_vector(31 downto 0);
  end record tracpu_data_o_t;

  type cache_modereg_reg_t is record
    ic0_en               : std_logic;
    dc0_en               : std_logic;
    ic0_inv              : std_logic;
    dc0_inv              : std_logic;
    ic1_en               : std_logic;
    dc1_en               : std_logic;
    ic1_inv              : std_logic;
    dc1_inv              : std_logic;
    int0                 : std_logic;
    int1                 : std_logic;
    cache01sel_ctrl_temp : std_logic; -- temporal. for experiment
    ddr_status_metas     : ddr_status_o_t;
    ddr_status_stable    : ddr_status_o_t;
  end record cache_modereg_reg_t;

  constant cachemodereg_reg_reset : cache_modereg_reg_t :=
  (
    '0',
    '0',
    '0',
    '0',
    '0',
    '0',
    '0',
    '0',
    '0',
    '0',
    '0',
    ((others => '0'), '0'),
    ((others => '0'), '0')
  );

  component icache_ram is port (
      clk125 : in    std_logic;
      clk200 : in    std_logic;
      rst    : in    std_logic;
      ra     : in    icache_ram_i_t;
      ry     : out   icache_ram_o_t
    );
  end component icache_ram;

  component icache is
    generic (
      mmu_arch : boolean := false
    );
    port (
      clk125 : in    std_logic;
      clk200 : in    std_logic;
      rst    : in    std_logic;
      -- ic on/off mode
      icccra : in    icccr_i_t;
      -- Cache RAM port
      ra : in    icache_ram_o_t;
      ry : out   icache_ram_i_t;
      -- CPU port
      a : in    icache_i_t;
      y : out   icache_o_t;
      -- DDR memory port
      ma : in    mem_i_t;
      my : out   mem_o_t
    );
  end component icache;

  component icache_ccl is
    generic (
      mmu_arch : boolean := false
    );
    port (
      clk : in    std_logic;
      rst : in    std_logic;
      -- Cache RAM port
      ra     : in    icache_ram_o_t;
      ry_ccl : out   icache_ramccl_i_t;
      -- CPU port
      a : in    icache_i_t;
      y : out   icache_o_t;
      -- Cclk Mclk if
      ctom : out   ctom_t;
      mtoc : in    mtoc_t;
      -- ic on/off mode
      icccra : in    icccr_i_t
    );
  end component icache_ccl;

  component icache_mcl is port (
      clk : in    std_logic;
      rst : in    std_logic;
      -- Cache RAM port
      ry_mcl : out   icache_rammcl_i_t;
      -- DDR memory port
      ma : in    mem_i_t;
      my : out   mem_o_t;
      -- Cclk Mclk if
      ctom : in    ctom_t;
      mtoc : out   mtoc_t
    );
  end component icache_mcl;

  component icache_modereg is port (
      rst : in    std_logic;
      clk : in    std_logic;
      -- cpu target port
      db_i : in    cpu_data_o_t;
      -- cpu target port
      db_o : out   cpu_data_i_t;
      -- ----------
      cpu0_ddr_ibus_o : in    cpu_instruction_o_t;
      cpu1_ddr_ibus_o : in    cpu_instruction_o_t;
      -- ----------
      cache0_ctrl_ic       : out   cache_ctrl_t;
      cache1_ctrl_ic       : out   cache_ctrl_t;
      cache0_ctrl_dc       : out   cache_ctrl_t;
      cache1_ctrl_dc       : out   cache_ctrl_t;
      ddr_status           : in    ddr_status_o_t;
      int0                 : out   std_logic;
      int1                 : out   std_logic;
      cache01sel_ctrl_temp : out   std_logic
    );
  end component icache_modereg;

  component icache_modereg_wsbu is port (
      rst : in    std_logic;
      clk : in    std_logic;
      --
      db_i : in    cpu_data_o_t;
      --
      db_o : out   cpu_data_i_t;
      --
      cpu0_ddr_ibus_o : in    cpu_instruction_o_t;
      cpu1_ddr_ibus_o : in    cpu_instruction_o_t;
      --
      ddr_status           : in    ddr_status_o_t;
      db_cctrans_o         : out   tracpu_data_o_t;
      cache01sel_ctrl_temp : out   std_logic
    );
  end component icache_modereg_wsbu;

  component dcache_ram is port (
      clk125 : in    std_logic;
      clk200 : in    std_logic;
      rst    : in    std_logic;
      ra     : in    dcache_ram_i_t;
      ry     : out   dcache_ram_o_t
    );
  end component dcache_ram;

  component dcache is
    generic (
      mmu_arch : boolean := false
    );
    port (
      clk125 : in    std_logic;
      clk200 : in    std_logic;
      rst    : in    std_logic;
      -- --------  dcache on/off mode  -----
      ctrl : in    cache_ctrl_t;
      -- --------  Cache RAM port ----------
      ra : in    dcache_ram_o_t;
      ry : out   dcache_ram_i_t;
      -- --------  CPU port ----------------
      a     : in    cpu_data_o_t;
      a_mmu : in    mmu_cache_i_t;
      lock  : in    std_logic;
      y     : out   cpu_data_i_t;
      -- --------  snoop port --------------
      sa : in    dcache_snoop_io_t;
      sy : out   dcache_snoop_io_t;
      -- --------  DDR memory port ---------
      ma : in    mem_i_t;
      my : out   mem_o_t
    );
  end component dcache;

  component dcache_ccl is
    generic (
      mmu_arch : boolean := false
    );
    port (
      clk : in    std_logic;
      rst : in    std_logic;
      -- Cache RAM port
      ra     : in    dcache_ram_o_t;
      ry_ccl : out   dcache_ramccl_i_t;
      -- CPU port
      a      : in    cpu_data_o_t;
      a_lock : in    std_logic;
      a_mmu  : in    mmu_cache_i_t;
      y      : out   cpu_data_i_t;
      -- snoop port
      sa : in    dcache_snoop_io_t;
      sy : out   dcache_snoop_io_t;
      -- Cclk Mclk if
      ctom : out   ctom_dc_t;
      mtoc : in    mtoc_dc_t;
      -- ic on/off mode
      ctrl : in    cache_ctrl_t
    );
  end component dcache_ccl;

  component dcache_mcl is port (
      clk : in    std_logic;
      rst : in    std_logic;
      -- Cache RAM port
      ry_mcl : out   dcache_rammcl_i_t;
      -- DDR memory port
      ma : in    mem_i_t;
      my : out   mem_o_t;
      -- Cclk Mclk if
      ctom : in    ctom_dc_t;
      mtoc : out   mtoc_dc_t
    );
  end component dcache_mcl;

  component icache_adapter is
    port (
      clk125 : in    std_logic;
      clk200 : in    std_logic;
      rst    : in    std_logic;
      ctrl   : in    cache_ctrl_t;
      ibus_o : in    cpu_instruction_o_t;
      ibus_i : out   cpu_instruction_i_t;

      dbus_o        : out   cpu_data_o_t;
      dbus_ddrburst : out   std_logic;
      dbus_i        : in    cpu_data_i_t;
      dbus_ack_r    : in    std_logic
    );
  end component icache_adapter;

  component dcache_adapter is
    port (
      clk125 : in    std_logic;
      clk200 : in    std_logic;
      rst    : in    std_logic;
      ctrl   : in    cache_ctrl_t;
      ibus_o : in    cpu_data_o_t;
      a_mmu  : in    mmu_cache_i_t := MMU_CACHE_I_RESET;
      lock   : in    std_logic;
      ibus_i : out   cpu_data_i_t;
      snpc_o : out   dcache_snoop_io_t;
      snpc_i : in    dcache_snoop_io_t;

      dbus_o        : out   cpu_data_o_t;
      dbus_lock     : out   std_logic;
      dbus_ddrburst : out   std_logic;
      dbus_i        : in    cpu_data_i_t;
      dbus_ack_r    : in    std_logic
    );
  end component dcache_adapter;

  component dcache_cacheable_mux is
    port (
      clk125       : in    std_logic;
      clk200       : in    std_logic;
      rst          : in    std_logic;
      ctrl         : in    cache_ctrl_t;
      cpu_o        : in    cpu_data_o_t;
      a_mmu        : in    mmu_cache_i_t;
      lock         : in    std_logic;
      cpu_i        : out   cpu_data_i_t;
      mem_o        : out   cpu_data_o_t;
      mem_lock     : out   std_logic;
      mem_ddrburst : out   std_logic;
      mem_i        : in    cpu_data_i_t;
      mem_ack_r    : in    std_logic
    );
  end component dcache_cacheable_mux;

end package cache_pack;

package body cache_pack is

  function itov (
    x,
    n : integer
  ) return std_logic_vector is
  begin

    return std_logic_vector(to_unsigned(x, n));

  end function itov;

  function vtoi (
    x : std_logic_vector
  ) return integer is

    variable v : std_logic_vector(x'high - x'low downto 0) := x;

  begin

    return to_integer(unsigned(v));

  end function vtoi;

  function to_cache_tag (
    a : std_logic_vector
  ) return std_logic_vector is

    variable ret : std_logic_vector(CACHE_TAG_WIDTH - 1 downto 0) := a(CACHE_REGION_WIDTH - 1 downto CACHE_INDEX_BITS + CACHE_LINE_WIDTH_BITS);

  begin

    return ret;

  end function to_cache_tag;

  function to_cache_valid (
    a : std_logic_vector
  ) return integer is
  begin

    return vtoi(a(CACHE_INDEX_BITS + CACHE_LINE_WIDTH_BITS - 1 downto CACHE_LINE_WIDTH_BITS + CACHE_VALID_BITS));

  end function to_cache_valid;

  function to_cache_vbit (
    a : std_logic_vector
  ) return integer is
  begin

    return vtoi(a(CACHE_LINE_WIDTH_BITS + CACHE_VALID_BITS - 1 downto CACHE_LINE_WIDTH_BITS));

  end function to_cache_vbit;

  function to_cache_index (
    a : std_logic_vector
  ) return integer is
  begin

    return vtoi(a(CACHE_INDEX_BITS + CACHE_LINE_WIDTH_BITS - 1 downto CACHE_LINE_WIDTH_BITS));

  end function to_cache_index;

  function to_cache_offset (
    a : std_logic_vector
  ) return integer is
  begin

    return vtoi(a(CACHE_LINE_WIDTH_BITS - 1 downto CACHE_MEM_WIDTH_BITS));

  end function to_cache_offset;

  function to_cache_addr (
    a : std_logic_vector
  ) return integer is
  begin

    return vtoi(a(CACHE_INDEX_BITS + CACHE_LINE_WIDTH_BITS - 1 downto CACHE_MEM_WIDTH_BITS));

  end function to_cache_addr;

  function to_cache_idata (
    a,
    d : std_logic_vector
  ) return std_logic_vector is
  begin

    if (a(CACHE_I_WIDTH_BITS) = '1') then
      return d(CACHE_I_WIDTH - 1               downto 0);
    else
      return d(CACHE_I_WIDTH + CACHE_I_WIDTH - 1 downto CACHE_I_WIDTH);
    end if;

  end function to_cache_idata;

  function is_cacheable (
    a : std_logic_vector(31 downto 0)
  ) return boolean is
  begin

    -- a(31:29): 000-011 = P0 (cached), 100 = P1 (cached), 101 = P2 (uncached),
    --           110 = P3 (cached), 111 = P4 (uncached).
    case a(31 downto 29) is

      when "101" =>

        return false;  -- P2

      when "111" =>

        return false;  -- P4

      when others =>

        return true;   -- P0, P1, P3

    end case;

  end function is_cacheable;

  function is_cacheable_mmu (
    a : std_logic_vector(31 downto 0);
    at : std_logic;
    c : std_logic
  ) return boolean is
  begin

    if (at = '1') then
      return c = '1';
    else
      return is_cacheable(a);
    end if;

  end function is_cacheable_mmu;

end package body cache_pack;
