configuration soc_cpus_config of cpus is
  for one_cpu_m0
    for all : cpu_core
      use entity work.cpu_core(arch);
      for arch
        for u_cpu : cpu
          use configuration work.cpu_synth_direct;
        end for;
      end for;
    end for;
  end for;
end configuration;
