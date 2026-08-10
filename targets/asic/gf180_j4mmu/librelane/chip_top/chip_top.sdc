# chip_top constraints.
#
# This file is PNR_SDC_FILE *and* SIGNOFF_SDC_FILE *and* FALLBACK_SDC_FILE, so
# it TOTALLY REPLACES LibreLane's base.sdc -- whatever is not written here is
# not constrained at all. Until 2026-08-09 this file was 6 lines (clock,
# propagated, uncertainty, reset false-path), which silently discarded every
# I/O, transition, capacitance, fanout, load and derate constraint that
# base.sdc would have applied from the PDK's own defaults. The resizer had
# nothing to close against, which is the leading suspect for the 9,626
# max-slew violations the 2026-08-09 chip_top run reported.
#
# Structure follows KianV's gf180mcu chip_top.sdc (same PDK, same Chip flow,
# same 9T/3.3V library, same 33 ns target) -- adapted to our pad set: our pads
# are individually named (pad_<signal>), not bidir_PAD[*]/input_PAD[*] arrays.
#
# The PDK's SCL config.tcl supplies MAX_TRANSITION_CONSTRAINT (3),
# MAX_FANOUT_CONSTRAINT (10) and MAX_CAPACITANCE_CONSTRAINT (0.2); the
# info-exists guards mirror base.sdc so a PDK that omits one is not a hard
# error.

current_design $::env(DESIGN_NAME)
set_units -time ns

# --- Clock ---------------------------------------------------------------
# The clock enters at bond pad clk_sys_PAD, through the in_c pad
# (pad_clk_sys) whose Y output drives the internal clk_sys net. Constrain the
# INTERNAL net, not the external port -- the pad cell has no timing arc we
# want in the clock definition.
create_clock -name clk_sys -period 33.0 [get_pins pad_clk_sys/Y]
set clocks [get_clocks clk_sys]

set_propagated_clock [all_clocks]
set_clock_uncertainty $::env(CLOCK_UNCERTAINTY_CONSTRAINT) $clocks
set_clock_transition $::env(CLOCK_TRANSITION_CONSTRAINT) $clocks

# --- Design-wide design rules -------------------------------------------
set_max_fanout $::env(MAX_FANOUT_CONSTRAINT) [current_design]

if { [info exists ::env(MAX_TRANSITION_CONSTRAINT)] } {
    set_max_transition $::env(MAX_TRANSITION_CONSTRAINT) [current_design]
}

if { [info exists ::env(MAX_CAPACITANCE_CONSTRAINT)] } {
    set_max_capacitance $::env(MAX_CAPACITANCE_CONSTRAINT) [current_design]
}

# --- I/O timing ----------------------------------------------------------
set input_delay_value  [expr $::env(CLOCK_PERIOD) * $::env(IO_DELAY_CONSTRAINT) / 100]
set output_delay_value [expr $::env(CLOCK_PERIOD) * $::env(IO_DELAY_CONSTRAINT) / 100]
puts "\[INFO] Setting input delay to: $input_delay_value"
puts "\[INFO] Setting output delay to: $output_delay_value"

# Input-only pads (gf180mcu_fd_io__in_c), EXCLUDING clk_sys_PAD (the clock
# source) and reset_PAD (asynchronous, false-pathed below).
set chip_input_ports [get_ports {
    uart0_rx_PAD
    spi2_miso_PAD
}]
set_input_delay -min 0                    -clock $clocks $chip_input_ports
set_input_delay -max $input_delay_value   -clock $clocks $chip_input_ports

# Bidirectional pads (gf180mcu_fd_io__bi_t): 81 cells over these 17 port
# names (gpio/qfl_io/sd_dq/sd_cmd_* are buses). Constrained in both
# directions -- the pad's direction is runtime state, not a static property.
set chip_bidir_ports [get_ports {
    gpio_PAD[*]
    qfl_cs_n_PAD
    qfl_io_PAD[*]
    qfl_sck_PAD
    sd_cmd_a_PAD[*]
    sd_cmd_ba_PAD[*]
    sd_cmd_cas_n_PAD
    sd_cmd_cke_PAD
    sd_cmd_cs_n_PAD
    sd_cmd_dqm_PAD[*]
    sd_cmd_ras_n_PAD
    sd_cmd_we_n_PAD
    sd_dq_PAD[*]
    spi2_clk_PAD
    spi2_cs_PAD
    spi2_mosi_PAD
    uart0_tx_PAD
}]
set_input_delay -min 0                    -clock $clocks $chip_bidir_ports
set_input_delay -max $input_delay_value   -clock $clocks $chip_bidir_ports
set_output_delay $output_delay_value      -clock $clocks $chip_bidir_ports

# --- Output load ---------------------------------------------------------
set cap_load [expr $::env(OUTPUT_CAP_LOAD) / 1000.0]
puts "\[INFO] Setting load to: $cap_load"
set_load $cap_load [all_outputs]

# --- Derating ------------------------------------------------------------
if { [info exists ::env(TIME_DERATING_CONSTRAINT)] } {
    puts "\[INFO] Setting timing derate to: $::env(TIME_DERATING_CONSTRAINT)%"
    set_timing_derate -early [expr 1 - [expr $::env(TIME_DERATING_CONSTRAINT) / 100]]
    set_timing_derate -late  [expr 1 + [expr $::env(TIME_DERATING_CONSTRAINT) / 100]]
}

# --- Exceptions ----------------------------------------------------------
# Asynchronous reset: no timing requirement on its arrival.
set_false_path -from [get_ports reset_PAD]
