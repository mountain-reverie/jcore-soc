-- GF180 vendor-macro configuration for the boot RAM (bootram_infer), mirroring
-- cache_gf180_config.vhd's pattern for the cache RAM: binds the entity to the
-- (gf180) architecture defined in bootram_2Nx8_gf180.vhd instead of the
-- default (inferred). This is an ADDITIONAL binding used only by the
-- P&R/area-metrics build path -- it does not change the default architecture
-- used by functional simulation / FPGA builds, which keep instantiating
-- bootram_infer(inferred) directly (see e.g. targets/asic/gf180_j4mmu's
-- cpus_xip_probe.vhd and the ulx3s boards).
configuration bootram_gf180 of bootram_infer is
  for gf180
  end for;
end configuration;
