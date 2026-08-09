set current_folder [file dirname [file normalize [info script]]]

# Placement site for core cells
# This can be found in the technology lef
set ::env(PLACE_SITE) "GF018hv5v_green_sc9"

# welltap and endcap cell
set ::env(WELLTAP_CELL) "$::env(STD_CELL_LIBRARY)__filltie"
set ::env(ENDCAP_CELL) "$::env(STD_CELL_LIBRARY)__endcap"

# defaults (can be overridden by designs):
# jcore-soc local compat shim: pdk_compat.migrate_old_config unconditionally
# reads config['SYNTH_DRIVING_CELL_PIN'] for any gf180mcu* PDK (config/
# pdk_compat.py "x2. Invalid Variables (gf180mcu)" block), but this ciel pin's
# config.tcl never defines a separate SYNTH_DRIVING_CELL_PIN (old combined
# "cell/pin" schema throughout) -- raised a bare KeyError. Define it so the
# compat shim can run.
#
# LibreLane 3.0.5 CHANGED what the shim then does with it, and the 2.4.x-era
# values below were silently wrong from the 3.0.5 bump (5c5e431) onward:
#   - 3.0.5 APPENDS SYNTH_DRIVING_CELL_PIN to SYNTH_DRIVING_CELL, so a value
#     that already carried "/ZN" resolved to "..._inv_1/ZN/ZN";
#   - 3.0.5 does NOT rebuild SYNTH_CLK_DRIVING_CELL from it (2.4.x did), so a
#     pinless "..._inv_4" stayed pinless.
# base.sdc then does `-pin [lindex [split $::env(SYNTH_CLK_DRIVING_CELL) "/"] 1]`
# -> empty -pin -> `set_driving_cell ... -pin "" [get_port clk]` -> OpenSTA
# reports `port '' not found`, attributed to base.sdc:59 (where that command's
# last argument sits). That killed STA Pre-PnR for every macro -- i.e. the whole
# die-area flow -- while looking like a clock-port problem. See
# docs/superpowers/specs/2026-08-08-gf180-die-ci-and-j4-core-metrics-design.md.
#
# So: give SYNTH_DRIVING_CELL the BARE cell (the shim appends the pin) and give
# SYNTH_CLK_DRIVING_CELL its pin explicitly, matching the upstream schema.
set ::env(SYNTH_DRIVING_CELL) "$::env(STD_CELL_LIBRARY)__inv_1"
if { ![info exist ::env(SYNTH_DRIVING_CELL_PIN)] } { set ::env(SYNTH_DRIVING_CELL_PIN) "ZN" }
set ::env(SYNTH_CLK_DRIVING_CELL) "$::env(STD_CELL_LIBRARY)__inv_4/ZN"

# update these
set ::env(OUTPUT_CAP_LOAD) "72.91" ; # femtofarad from pin I in liberty file
set ::env(SYNTH_BUFFER_CELL) "$::env(STD_CELL_LIBRARY)__buf_1/I/Z"
set ::env(SYNTH_TIEHI_CELL) "$::env(STD_CELL_LIBRARY)__tieh/Z"
set ::env(SYNTH_TIELO_CELL) "$::env(STD_CELL_LIBRARY)__tiel/ZN"

# Fillcell insertion
set ::env(FILL_CELLS) "$::env(STD_CELL_LIBRARY)__fill_*"
set ::env(DECAP_CELLS) "$::env(STD_CELL_LIBRARY)__fillcap_*"

# Diode Insertion
set ::env(DIODE_CELL) "$::env(STD_CELL_LIBRARY)__antenna/I"

set ::env(CELL_PAD_EXCLUDE) "$::env(STD_CELL_LIBRARY)__filltie $::env(STD_CELL_LIBRARY)__fill_* $::env(STD_CELL_LIBRARY)__endcap"

# TritonCTS configurations
set ::env(CTS_ROOT_BUFFER) "$::env(STD_CELL_LIBRARY)__clkbuf_16"
set ::env(CTS_CLK_BUFFERS) "$::env(STD_CELL_LIBRARY)__clkbuf_2 $::env(STD_CELL_LIBRARY)__clkbuf_4 $::env(STD_CELL_LIBRARY)__clkbuf_8"

set ::env(PDN_RAIL_WIDTH) 0.6
# jcore-soc local compat shim: see top-level config.tcl FP_PDN_* alias block.
set ::env(FP_PDN_RAIL_WIDTH) $::env(PDN_RAIL_WIDTH)

# The library maximum transition is 8.9ns; setting it to lower value
set ::env(MAX_TRANSITION_CONSTRAINT) 3
set ::env(MAX_FANOUT_CONSTRAINT) 10
set ::env(MAX_CAPACITANCE_CONSTRAINT) 0.2

set ::env(GPL_CELL_PADDING) {0}
set ::env(DPL_CELL_PADDING) {0}

set ::env(TRISTATE_CELLS) "$::env(STD_CELL_LIBRARY)__bufz*"

set ::env(SYNTH_CLOCKGATE_POSEDGE_ICG) "$::env(STD_CELL_LIBRARY)__icgtp_1/E/CLK/Q"
set ::env(SYNTH_CLOCKGATE_NEGEDGE_ICG) "$::env(STD_CELL_LIBRARY)__icgtn_1/E/CLKN/Q"
