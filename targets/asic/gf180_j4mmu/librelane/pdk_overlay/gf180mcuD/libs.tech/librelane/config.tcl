# Process node
set ::env(PROCESS) 180
set ::env(DEF_UNITS_PER_MICRON) 2000

set ::env(VDD_PIN) "VDD"
set ::env(GND_PIN) "VSS"
set ::env(VDD_PIN_VOLTAGE) "3.30"
set ::env(GND_PIN_VOLTAGE) "0.00"

set ::env(SCL_POWER_PINS) "VDD VNW"
set ::env(SCL_GROUND_PINS) "VSS VPW"

if { ![info exist ::env(STD_CELL_LIBRARY)] } {
    set ::env(STD_CELL_LIBRARY) gf180mcu_fd_sc_mcu7t5v0
}

if { ![info exist ::env(PAD_CELL_LIBRARY)] } {
    set ::env(PAD_CELL_LIBRARY) gf180mcu_fd_io
}

# Technology lib

# Corners


set ::env(TIMING_VIOLATION_CORNERS) "*tt*"

# Technology LEF
# jcore-soc local compat shim: LibreLane 2.4.x's Tcl->Python config bridge
# (librelane/common/tcl.py TclUtils._eval_env) only captures mutations that
# flow through the (renamed/intercepted) Tcl `set` command; `dict set
# ::env(KEY) ...` mutates the interpreter's real ::env(KEY) variable (a
# plain sourced Tcl script sees the fully-populated dict, as verified with a
# standalone tclsh) but is invisible to that capture shim, so KEY resolves
# to an EMPTY dict on the Python side. Every `dict set ::env(FOO) ...` below
# is therefore rewritten as one atomic `set ::env(FOO) [dict create ...]`
# call so the whole dict is visible to the capture shim.
set ::env(TECH_LEFS) [dict create \
    nom_* [glob "$::env(PDK_ROOT)/$::env(PDK)/libs.ref/$::env(STD_CELL_LIBRARY)/techlef/*__nom.tlef"] \
    min_* [glob "$::env(PDK_ROOT)/$::env(PDK)/libs.ref/$::env(STD_CELL_LIBRARY)/techlef/*__min.tlef"] \
    max_* [glob "$::env(PDK_ROOT)/$::env(PDK)/libs.ref/$::env(STD_CELL_LIBRARY)/techlef/*__max.tlef"] \
]

# Standard cells
set ::env(CELL_LEFS) [glob "$::env(PDK_ROOT)/$::env(PDK)/libs.ref/$::env(STD_CELL_LIBRARY)/lef/*.lef"]
set ::env(CELL_GDS) [glob "$::env(PDK_ROOT)/$::env(PDK)/libs.ref/$::env(STD_CELL_LIBRARY)/gds/*.gds"]
set ::env(CELL_VERILOG_MODELS) "$::env(PDK_ROOT)/$::env(PDK)/libs.ref/$::env(STD_CELL_LIBRARY)/verilog/$::env(STD_CELL_LIBRARY).v"
set ::env(CELL_SPICE_MODELS) "$::env(PDK_ROOT)/$::env(PDK)/libs.ref/$::env(STD_CELL_LIBRARY)/spice/$::env(STD_CELL_LIBRARY).spice"
set ::env(CELL_CDLS)	"$::env(PDK_ROOT)/$::env(PDK)/libs.ref/$::env(STD_CELL_LIBRARY)/cdl/$::env(STD_CELL_LIBRARY).cdl"

# Pad cells
set ::env(PAD_LEFS) [glob "$::env(PDK_ROOT)/$::env(PDK)/libs.ref/$::env(PAD_CELL_LIBRARY)/lef/*.lef"]
# The foundry library must be read in before the ef library because of ghost cells (references)
set ::env(PAD_GDS) "\
    $::env(PDK_ROOT)/$::env(PDK)/libs.ref/$::env(PAD_CELL_LIBRARY)/gds/gf180mcu_fd_io.gds\
    $::env(PDK_ROOT)/$::env(PDK)/libs.ref/$::env(PAD_CELL_LIBRARY)/gds/gf180mcu_ef_io.gds\
"
set ::env(PAD_VERILOG_MODELS) [glob "$::env(PDK_ROOT)/$::env(PDK)/libs.ref/$::env(PAD_CELL_LIBRARY)/verilog/*__blackbox_pp.v"]
set ::env(PAD_SPICE_MODELS) [glob "$::env(PDK_ROOT)/$::env(PDK)/libs.ref/$::env(PAD_CELL_LIBRARY)/spice/*.spice"]
set ::env(PAD_CDLS) "$::env(PDK_ROOT)/$::env(PDK)/libs.ref/$::env(PAD_CELL_LIBRARY)/cdl/$::env(PAD_CELL_LIBRARY).cdl"

# Latch mapping
set ::env(SYNTH_LATCH_MAP) "$::env(PDK_ROOT)/$::env(PDK)/libs.tech/librelane/$::env(STD_CELL_LIBRARY)/latch_map.v"

# Tri-state buffer mapping
set ::env(TRISTATE_BUFFER_MAP) "$::env(PDK_ROOT)/$::env(PDK)/libs.tech/librelane/$::env(STD_CELL_LIBRARY)/tribuff_map.v"

# Full adder mapping
set ::env(FULL_ADDER_MAP) "$::env(PDK_ROOT)/$::env(PDK)/libs.tech/librelane/$::env(STD_CELL_LIBRARY)/fa_map.v"

# Ripple carry adder mapping
set ::env(RIPPLE_CARRY_ADDER_MAP) "$::env(PDK_ROOT)/$::env(PDK)/libs.tech/librelane/$::env(STD_CELL_LIBRARY)/rca_map.v"

# Carry select adder mapping
set ::env(CARRY_SELECT_ADDER_MAP) "$::env(PDK_ROOT)/$::env(PDK)/libs.tech/librelane/$::env(STD_CELL_LIBRARY)/csa_map.v"

# Default No Synth List
set ::env(SYNTH_EXCLUDED_CELL_FILE) "$::env(PDK_ROOT)/$::env(PDK)/libs.tech/librelane/$::env(STD_CELL_LIBRARY)/synth_exclude.cells"

# Default DRC Exclude List
set ::env(PNR_EXCLUDED_CELL_FILE) "$::env(PDK_ROOT)/$::env(PDK)/libs.tech/librelane/$::env(STD_CELL_LIBRARY)/pnr_exclude.cells"

# Open-RCX Rules File
set ::env(RCX_RULES) "$::env(PDK_ROOT)/$::env(PDK)/libs.tech/librelane/rcx_rules.info"

# Floorplanning

# I/O Layer info
set ::env(IO_PIN_H_LAYER) "Metal3"
set ::env(IO_PIN_V_LAYER) "Metal2"

# PDN Macro blockages list
set ::env(MACRO_BLOCKAGES_LAYER) "Metal1 Metal2 Metal3 Metal4 Metal5"

## Tap Cell Dist
set ::env(FP_TAPCELL_DIST) 20

# Extra PDN configs
set ::env(PDN_RAIL_LAYER) "Metal1"
set ::env(PDN_RAIL_OFFSET) 0

set ::env(PDN_VERTICAL_LAYER) "Metal4"
set ::env(PDN_HORIZONTAL_LAYER) "Metal5"

set ::env(PDN_VWIDTH) 1.6
set ::env(PDN_HWIDTH) 1.6
set ::env(PDN_VSPACING) 1.7
set ::env(PDN_HSPACING) 1.7
set ::env(PDN_VOFFSET) 16.32
set ::env(PDN_VPITCH) 153.6
set ::env(PDN_HOFFSET) 16.65
set ::env(PDN_HPITCH) 153.18

## Core Ring PDN defaults
set ::env(PDN_CORE_RING_VWIDTH) 1.6
set ::env(PDN_CORE_RING_HWIDTH) 1.6
set ::env(PDN_CORE_RING_VSPACING) 1.7
set ::env(PDN_CORE_RING_HSPACING) 1.7
set ::env(PDN_CORE_RING_VOFFSET) 6
set ::env(PDN_CORE_RING_HOFFSET) 6

# Timing
set ::env(RCX_RULES) "$::env(PDK_ROOT)/$::env(PDK)/libs.tech/librelane/rules.openrcx.$::env(PDK).nom"
set ::env(RCX_RULES_MIN) "$::env(PDK_ROOT)/$::env(PDK)/libs.tech/librelane/rules.openrcx.$::env(PDK).min"
set ::env(RCX_RULES_MAX) "$::env(PDK_ROOT)/$::env(PDK)/libs.tech/librelane/rules.openrcx.$::env(PDK).max"

# Routing
set ::env(METAL_LAYER_NAMES) "Metal1 Metal2 Metal3 Metal4 Metal5"
set ::env(RT_MIN_LAYER) "Metal2" ;# stdcells heavily use Metal1 - setting it to Metal1 will cause congestions
set ::env(RT_MAX_LAYER) "Metal5"
set ::env(DRT_MIN_LAYER) "Metal1"
set ::env(GRT_LAYER_ADJUSTMENTS) "0,0,0,0,0"

## Tracks info
set ::env(FP_TRACKS_INFO) "$::env(PDK_ROOT)/$::env(PDK)/libs.tech/librelane/$::env(STD_CELL_LIBRARY)/tracks.info"

# Signoff
## Magic
set ::env(MAGICRC) "$::env(PDK_ROOT)/$::env(PDK)/libs.tech/magic/$::env(PDK).magicrc"
set ::env(MAGIC_TECH) "$::env(PDK_ROOT)/$::env(PDK)/libs.tech/magic/$::env(PDK).tech"

## Klayout
set ::env(KLAYOUT_TECH) "$::env(PDK_ROOT)/$::env(PDK)/libs.tech/klayout/tech/gf180mcu.lyt"
set ::env(KLAYOUT_PROPERTIES) "$::env(PDK_ROOT)/$::env(PDK)/libs.tech/klayout/tech/gf180mcu.lyp"
set ::env(KLAYOUT_DEF_LAYER_MAP) "$::env(PDK_ROOT)/$::env(PDK)/libs.tech/klayout/tech/gf180mcu.map"

set ::env(KLAYOUT_DRC_RUNSET) "$::env(PDK_ROOT)/$::env(PDK)/libs.tech/klayout/tech/drc/gf180mcu.drc"
set ::env(KLAYOUT_DRC_OPTIONS) [dict create]
dict set ::env(KLAYOUT_DRC_OPTIONS) variant $::env(PDK)
dict set ::env(KLAYOUT_DRC_OPTIONS) decks "all,-antenna,-density"

set ::env(KLAYOUT_DENSITY_RUNSET) "$::env(PDK_ROOT)/$::env(PDK)/libs.tech/klayout/tech/drc/gf180mcu.drc"
set ::env(KLAYOUT_DENSITY_OPTIONS) [dict create]
dict set ::env(KLAYOUT_DENSITY_OPTIONS) variant $::env(PDK)
dict set ::env(KLAYOUT_DENSITY_OPTIONS) decks "density"

set ::env(KLAYOUT_ANTENNA_RUNSET) "$::env(PDK_ROOT)/$::env(PDK)/libs.tech/klayout/tech/drc/gf180mcu.drc"
set ::env(KLAYOUT_ANTENNA_OPTIONS) [dict create]
dict set ::env(KLAYOUT_ANTENNA_OPTIONS) variant $::env(PDK)
dict set ::env(KLAYOUT_ANTENNA_OPTIONS) decks "antenna"

set ::env(KLAYOUT_FILLER_SCRIPT) "$::env(PDK_ROOT)/$::env(PDK)/libs.tech/klayout/tech/scripts/fill_all.rb"
set ::env(KLAYOUT_FILLER_OPTIONS) [dict create]

set ::env(KLAYOUT_LVS_SCRIPT) "$::env(PDK_ROOT)/$::env(PDK)/libs.tech/klayout/tech/lvs/gf180mcu.lvs"
set ::env(KLAYOUT_LVS_OPTIONS) [dict create run_mode deep]

set ::env(KLAYOUT_SEALRING_SCRIPT) "$::env(PDK_ROOT)/$::env(PDK)/libs.tech/klayout/tech/scripts/sealring.py"

## Netgen
set ::env(NETGEN_SETUP) "$::env(PDK_ROOT)/$::env(PDK)/libs.tech/netgen/$::env(PDK)_setup.tcl"

# Used for parasitics estimation, IR drop analysis, etc
set ::env(LAYERS_RC) [dict create]

# RC fit from OpenROAD
# https://github.com/The-OpenROAD-Project/OpenROAD-flow-scripts/blob/master/flow/platforms/gf180/setRC.tcl
dict set ::env(LAYERS_RC) "*" Metal2 res 3.85861E-04
dict set ::env(LAYERS_RC) "*" Metal2 cap 1.35357E-04
dict set ::env(LAYERS_RC) "*" Metal3 res 2.06673E-04
dict set ::env(LAYERS_RC) "*" Metal3 cap 1.46141E-04
dict set ::env(LAYERS_RC) "*" Metal4 res 1.68609E-04
dict set ::env(LAYERS_RC) "*" Metal4 cap 1.50688E-04
dict set ::env(LAYERS_RC) "*" Metal5 res 7.92778E-05
dict set ::env(LAYERS_RC) "*" Metal5 cap 1.55595E-04

set ::env(VIAS_R) [dict create]

# Best case (and used for nom)
dict set ::env(VIAS_R) "*" Via1 res 4.23
dict set ::env(VIAS_R) "*" Via2 res 4.23
dict set ::env(VIAS_R) "*" Via3 res 4.23
dict set ::env(VIAS_R) "*" Via4 res 4.23

# Worst case (last one wins)
dict set ::env(VIAS_R) "max_*" Via1 res 16.845
dict set ::env(VIAS_R) "max_*" Via2 res 16.845
dict set ::env(VIAS_R) "max_*" Via3 res 16.845
dict set ::env(VIAS_R) "max_*" Via4 res 16.845

set ::env(SIGNAL_WIRE_RC_LAYERS) "Metal2 Metal3 Metal4"
set ::env(CLOCK_WIRE_RC_LAYERS) "Metal2 Metal3 Metal4"

# Base SDC

# in ns
set ::env(CLOCK_UNCERTAINTY_CONSTRAINT) 0.25
set ::env(CLOCK_TRANSITION_CONSTRAINT) 0.15

# Percentage
set ::env(TIME_DERATING_CONSTRAINT) 5
set ::env(IO_DELAY_CONSTRAINT) 20

# --- jcore-soc local compat shim (added, not from upstream PDK release) ---
# LibreLane 2.4.x's pdk_compat.migrate_old_config() unconditionally `del`s
# these three keys for any PDK starting with "gf180mcu" (config/pdk_compat.py
# "x2. Invalid Variables (gf180mcu)" block), assuming a newer/different
# gf180mcu PDK config.tcl schema defines them. This ciel pin
# (f6eeac7dad085ffcc829ccfd721f7b4ce39edcf7) does not define them at all,
# which raised a bare KeyError and aborted config load before any flow step
# ran. We are not using the GPIO pad ring or the KLayout DRC tech script for
# this logic-only smoke macro, so empty placeholders are sufficient to let
# LibreLane's compat shim proceed; see targets/asic/gf180_j4mmu/librelane/
# run.sh / README for the smoke-test context in jcore-soc.
if { ![info exist ::env(GPIO_PADS_LEF)] } { set ::env(GPIO_PADS_LEF) "" }
if { ![info exist ::env(GPIO_PADS_VERILOG)] } { set ::env(GPIO_PADS_VERILOG) "" }
if { ![info exist ::env(KLAYOUT_DRC_TECH_SCRIPT)] } { set ::env(KLAYOUT_DRC_TECH_SCRIPT) "" }

# jcore-soc local compat shim: LibreLane 2.4.x's PDN config variables are all
# FP_PDN_*-prefixed; this ciel pin's config.tcl still defines the old
# (pre-prefix) PDN_* names only. Alias them 1:1 rather than renaming the
# originals (other tech scripts in this PDK build still read the bare
# PDN_* names).
set ::env(FP_PDN_RAIL_LAYER) $::env(PDN_RAIL_LAYER)
set ::env(FP_PDN_VERTICAL_LAYER) $::env(PDN_VERTICAL_LAYER)
set ::env(FP_PDN_HORIZONTAL_LAYER) $::env(PDN_HORIZONTAL_LAYER)
set ::env(FP_PDN_VWIDTH) $::env(PDN_VWIDTH)
set ::env(FP_PDN_HWIDTH) $::env(PDN_HWIDTH)
set ::env(FP_PDN_VSPACING) $::env(PDN_VSPACING)
set ::env(FP_PDN_HSPACING) $::env(PDN_HSPACING)
set ::env(FP_PDN_VOFFSET) $::env(PDN_VOFFSET)
set ::env(FP_PDN_VPITCH) $::env(PDN_VPITCH)
set ::env(FP_PDN_HOFFSET) $::env(PDN_HOFFSET)
set ::env(FP_PDN_HPITCH) $::env(PDN_HPITCH)
set ::env(FP_PDN_CORE_RING_VWIDTH) $::env(PDN_CORE_RING_VWIDTH)
set ::env(FP_PDN_CORE_RING_HWIDTH) $::env(PDN_CORE_RING_HWIDTH)
set ::env(FP_PDN_CORE_RING_VSPACING) $::env(PDN_CORE_RING_VSPACING)
set ::env(FP_PDN_CORE_RING_HSPACING) $::env(PDN_CORE_RING_HSPACING)
set ::env(FP_PDN_CORE_RING_VOFFSET) $::env(PDN_CORE_RING_VOFFSET)
set ::env(FP_PDN_CORE_RING_HOFFSET) $::env(PDN_CORE_RING_HOFFSET)
set ::env(FP_PDN_RAIL_OFFSET) $::env(PDN_RAIL_OFFSET)
set ::env(FP_IO_HLAYER) $::env(IO_PIN_H_LAYER)
set ::env(FP_IO_VLAYER) $::env(IO_PIN_V_LAYER)
if { ![info exist ::env(FILL_CELL)] } { set ::env(FILL_CELL) $::env(STD_CELL_LIBRARY)__fill_1 }
if { ![info exist ::env(DECAP_CELL)] } { set ::env(DECAP_CELL) $::env(STD_CELL_LIBRARY)__fillcap_4 }
# jcore-soc local compat shim: pdk_compat.migrate_old_config's gf180mcu/
# sky130 branch (`"LIB" not in config`) expects the old single-string
# LIB_SYNTH/LIB_SLOWEST/LIB_FASTEST variables (one liberty path each, used to
# derive the per-corner LIB dict + STA_CORNERS); this ciel pin only defines
# the newer-looking CELL_LIBS dict (never consumed by this compat path).
# Point all three at the single tt/ff/ss corner libs already used above.
