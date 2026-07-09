-- Direct-elaboration configuration binding entity cpus's cpus_coremark
-- architecture to the J1 core (mirrors cpus_config.vhd's binding of
-- one_cpu_ebr, used by soc_gen; this variant is hand-maintained for the
-- direct-GHDL smoke test in tb/cpus_coremark_boot_tb.vhd, since Task 7b does
-- not go through soc_gen).
configuration cpus_coremark_config of cpus is
  for cpus_coremark
    for all : cpu_core
      use entity work.cpu_core(arch);
      for arch
        for u_cpu : cpu
          use configuration work.cpu_synth_j1_dsp;
        end for;
      end for;
    end for;
  end for;
end configuration;
