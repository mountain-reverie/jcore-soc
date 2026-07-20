-- GF180 vendor-macro configurations for the cache tag/data RAM, mirroring
-- components/cpu/cache/cache_config_fpga.vhd's icache_ram_infer/
-- icache_adapter_fpga (and dcache equivalents) but binding ram_1rw/ram_2rw to
-- the tech/gf180 vendor SRAM macros (via ram_1rw_gf180/ram_2rw_gf180 from
-- mem_gf180_config.vhd) instead of (inferred). icache_ram/dcache_ram's single
-- architecture is `beh`; within it the tag RAM instances (ram_1rw) are direct
-- component instantiations and the data RAM instances (ram_2rw) sit inside a
-- generate block labeled `ram` -- see components/cpu/cache/icache_ram.vhd /
-- dcache_ram.vhd. Not an edit to that RTL: these configurations only bind
-- entities it already declares.
configuration icache_ram_gf180 of icache_ram is
  use work.memory_pack.all;
  for beh
    for all : ram_1rw
      use configuration work.ram_1rw_gf180;
    end for;
    for ram
      for all : ram_2rw
        use configuration work.ram_2rw_gf180;
      end for;
    end for;
  end for;
end configuration;

configuration icache_adapter_gf180 of icache_adapter is
  for arch
    for all : icache_ram
      use configuration work.icache_ram_gf180;
    end for;
  end for;
end configuration;

configuration dcache_ram_gf180 of dcache_ram is
  use work.memory_pack.all;
  for beh
    for all : ram_1rw
      use configuration work.ram_1rw_gf180;
    end for;
    for ram
      for all : ram_2rw
        use configuration work.ram_2rw_gf180;
      end for;
    end for;
  end for;
end configuration;

configuration dcache_adapter_gf180 of dcache_adapter is
  for arch
    for all : dcache_ram
      use configuration work.dcache_ram_gf180;
    end for;
  end for;
end configuration;
