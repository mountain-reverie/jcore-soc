-- Hand-written board config for the ULX3S M0 target. On the soc_gen boards
-- this package is generated; M0's top is hand-written, so we supply a minimal
-- one. cpu_core/cpu_core_pkg only need work.config to exist (they reference no
-- CFG_ constant); the clock constants below document the M0 ~25 MHz CPU clock.
package config is
  constant CFG_CLK_CPU_PERIOD_NS : integer := 40;  -- 25 MHz CPU clock
  constant CFG_CLK_CPU_FREQ_HZ   : integer := 25000000;
end package config;
