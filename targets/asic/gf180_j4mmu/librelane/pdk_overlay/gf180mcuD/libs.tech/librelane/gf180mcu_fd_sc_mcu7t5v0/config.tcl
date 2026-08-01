set current_folder [file dirname [file normalize [info script]]]

# Placement site for core cells
# This can be found in the technology lef
set ::env(PLACE_SITE) "GF018hv5v_mcu_sc7"

# welltap and endcap cell
set ::env(WELLTAP_CELL) "$::env(STD_CELL_LIBRARY)__filltie"
set ::env(ENDCAP_CELL) "$::env(STD_CELL_LIBRARY)__endcap"

# defaults (can be overridden by designs):
set ::env(SYNTH_DRIVING_CELL) "$::env(STD_CELL_LIBRARY)__inv_1/ZN"
# jcore-soc local compat shim: LibreLane 2.4.x's pdk_compat.migrate_old_config
# unconditionally reads config['SYNTH_DRIVING_CELL_PIN'] to rebuild
# SYNTH_CLK_DRIVING_CELL for any gf180mcu* PDK (config/pdk_compat.py "x2.
# Invalid Variables (gf180mcu)" block), but this ciel pin's config.tcl never
# defines a separate SYNTH_DRIVING_CELL_PIN (old combined "cell/pin" schema
# throughout) -- raised a bare KeyError. Define it so the compat shim can run.
if { ![info exist ::env(SYNTH_DRIVING_CELL_PIN)] } { set ::env(SYNTH_DRIVING_CELL_PIN) "ZN" }
set ::env(SYNTH_CLK_DRIVING_CELL) "$::env(STD_CELL_LIBRARY)__inv_4"

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
