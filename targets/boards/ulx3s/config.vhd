-- Hand-written board config for the ULX3S M0 target. On the soc_gen boards
-- this package is generated; M0's top is hand-written, so we supply a minimal
-- one. cpu_core/cpu_core_pkg only need work.config to exist (they reference no
-- CFG_ constant); the clock constants below document the M0 ~25 MHz CPU clock.
package config is
  constant CFG_CLK_CPU_PERIOD_NS : integer := 50;  -- 20 MHz CPU clock (M2: full
  constant CFG_CLK_CPU_FREQ_HZ   : integer := 20000000;  -- SoC closes ~22.4 MHz)
  -- M1b: ddrc_cnt_pkg (imported by cache_pkg for ddr_status_o_t) needs this.
  -- We use sdram_ctrl, not the ddr2 fsm, so the exact value is inert here.
  constant CFG_DDR_CK_CYCLE      : integer := 20;
end package config;
