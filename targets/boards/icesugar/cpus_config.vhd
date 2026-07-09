configuration soc_cpus_config of cpus is
  for one_cpu_xip
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
