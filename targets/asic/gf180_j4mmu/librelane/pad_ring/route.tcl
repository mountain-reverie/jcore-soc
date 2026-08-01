# Direct-OpenROAD finish for the flat pad_ring: global + detailed route on the
# post-CTS odb that LibreLane produced (run.sh macro=pad_ring, OL_TO=OpenROAD.CTS).
#
# Why direct OpenROAD instead of LibreLane's route steps: the flat integration
# is congested enough that GRT reports GRT-0118, which LibreLane's grt.tcl
# treats as a hard error. `global_route -allow_congestion` pushes past it and
# detailed_route then resolves the overflow (final result: 0 DRC).
#
# Invoke inside the librelane docker (repo mounted at /work), after reading the
# post-CTS odb + the tech/std-cell/macro LEFs -- see README.md for the full
# read_* preamble. Output paths default to the CWD; override with PADRING_DEF /
# PADRING_DRC env vars.

set out_def [expr {[info exists ::env(PADRING_DEF)] ? $::env(PADRING_DEF) : "routed_flat.def"}]
set drc_rpt [expr {[info exists ::env(PADRING_DRC)] ? $::env(PADRING_DRC) : "flatpr_drc.rpt"}]
set out_json [expr {[info exists ::env(PADRING_JSON)] ? $::env(PADRING_JSON) : "padring_metrics.json"}]

set blk [[[ord::get_db] getChip] getBlock]
set nsig 0; set nspec 0; set ndisc 0
foreach net [$blk getNets] {
  set n [$net getName]; set st [$net getSigType]
  # *_PAD chip terminals + power/ground are strapped by pad abutment, not routed
  if {[string match "*_PAD" $n] || $st=="POWER" || $st=="GROUND"} {
    $net setSpecial; incr nspec
  } else { incr nsig }
}
puts ">>> FLAT route: $nsig signal nets, $nspec special (PAD/power/ties)"
global_route -allow_congestion -congestion_iterations 100
puts ">>> global route done"
# 3.0.5 TritonRoute (stricter than 2.4.2) computes pin access even for special
# nets and hard-errors (DRT-0073) on the pad PAD terminals at the die edge,
# which it cannot create an access point for. Those *_PAD nets are external chip
# terminals (pad<->top-port, strapped by abutment, never internally routed), so
# disconnect their instance terminals before detailed routing -- the internal
# soc<->pad core-side nets (A/Y/OE/IE) still route normally.
foreach net [$blk getNets] {
  if {[string match "*_PAD" [$net getName]]} {
    foreach iterm [$net getITerms] { odb::dbITerm_disconnect $iterm; incr ndisc }
  }
}
puts ">>> disconnected $ndisc PAD-terminal iterms (external chip pads)"
detailed_route -output_drc $drc_rpt -verbose 1
puts ">>> detailed route done"
write_def $out_def
puts ">>> wrote $out_def"

# emit a small metrics JSON for the CI die-area benchmark: padded die area
# (from the block die bbox) + detailed-route DRC count (from the drc report).
set die [$blk getDieArea]
set dbu [[$blk getTech] getDbUnitsPerMicron]
set dw [expr {double([$die dx])/$dbu}]
set dh [expr {double([$die dy])/$dbu}]
set ndrc 0
if {[file exists $drc_rpt]} {
  set fh [open $drc_rpt r]
  while {[gets $fh line] >= 0} { if {[string match "*violation*" [string tolower $line]]} { incr ndrc } }
  close $fh
}
set jf [open $out_json w]
puts $jf [format "{\"design__die__width_um\": %.2f, \"design__die__height_um\": %.2f, \"design__die__area\": %.1f, \"design__route__drc_errors\": %d, \"design__route__wire_segments\": %d}" \
  $dw $dh [expr {$dw*$dh}] $ndrc $nsig]
close $jf
puts ">>> wrote $out_json (die ${dw}x${dh} um, $ndrc DRC)"
