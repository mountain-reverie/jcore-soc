# chip_top constraints. The clock enters at bond pad clk_sys_PAD, through the
# in_c pad (pad_clk_sys) whose Y output drives the internal clk_sys net.
create_clock -name clk_sys -period 33.0 [get_pins pad_clk_sys/Y]
set_propagated_clock [all_clocks]
set_clock_uncertainty 0.25 [all_clocks]
set_false_path -from [get_ports reset_PAD]
