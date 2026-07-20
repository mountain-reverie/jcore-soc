-- GF180 vendor-macro configurations for the generic ram_1rw/ram_2rw memory
-- interfaces (see lib/memory_tech_lib/tech/sim/mem_sim_config.vhd for the
-- (sim) counterpart this mirrors). Only the two macros with a real tech/gf180
-- vendor arch (ram_2x8x256_1rw and ram_2x8x2048_2rw -- the cache tag/data
-- macros) are routed to (gf180); the other genram branches (ram_18x2048_1rw,
-- ram_32x1x512_2rw) are dead for cache use but must still resolve to an
-- existing architecture, so they stay bound to (sim).
configuration ram_1rw_gf180 of ram_1rw is
  for memories
    for rows
      for genram_18x2048
        for all : ram_18x2048_1rw
          use entity work.ram_18x2048_1rw(sim);
        end for;
      end for;
      for genram_2x8x256
        for all : ram_2x8x256_1rw
          use entity work.ram_2x8x256_1rw(gf180);
        end for;
      end for;
    end for;
  end for;
end configuration;

configuration ram_2rw_gf180 of ram_2rw is
  for memories
    for rows
      for genram_32x1x512
        for all : ram_32x1x512_2rw
          use entity work.ram_32x1x512_2rw(sim);
        end for;
      end for;
      for genram_2x8x2048
        for all : ram_2x8x2048_2rw
          use entity work.ram_2x8x2048_2rw(gf180);
        end for;
      end for;
    end for;
  end for;
end configuration;
