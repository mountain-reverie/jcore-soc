# nextpnr-ecp5 --pre-place floorplan for the ULX3S dual-core variants.
#
# The dual-core (j2-dual / j4-dual) Fmax is limited by a half-cycle
# path on sdram_clk (shared_ram is clocked on the falling edge):
#
#   coreN.data_master -> core0/core1 u_datapath.mem[lock]
#                     -> cpumreg (shared-RAM lock arbiter)
#                     -> ddr_ram_mux.u_dcache0
#                     -> shared_ram.CEB   (block-RAM clock-enable)
#
# On the -6 85F the router spreads this cone across the whole die (source and
# sink ~80 columns apart), so ~78% of the delay is routing, not logic. We pull
# the SHARED coupling logic -- the lock arbiter (cpumreg), the shared RAM, the
# dcache on the path, and each core's mem[lock] datapath FFs -- into one compact
# central region so those long nets get short. The bulk of each core stays
# unconstrained, so this de-congests the path without caging the whole design
# (the cluster is ~85% of all cells; caging it all just reproduces full-die
# placement and helps nothing).
#
# Measured on j4-dual (commit under test): 23.83 MHz -> 25.18 MHz (+5.7%),
# well above the +/-0.5 MHz seed noise. This is a placement-only, no-RTL change;
# the real structural fix is to pipeline the arbiter path so it is no longer a
# half-cycle constraint.
#
# Applied only for *-dual variants (see synth.sh); harmless if a matched module
# is absent (constrainCellToRegion just matches fewer cells).

# Fabric extent (85F: ~126 x 95 in nextpnr location coords).
maxx = maxy = 0
for bel in ctx.getBels():
    loc = ctx.getBelLocation(bel)
    if loc.x > maxx:
        maxx = loc.x
    if loc.y > maxy:
        maxy = loc.y

# Central band: middle ~40% in x, full height -- generous so it never runs out
# of slices for the ~3.8k coupling cells.
x0 = int(maxx * 0.30)
x1 = int(maxx * 0.70)
ctx.createRectangularRegion("lockcluster", x0, 0, x1, maxy)


def on_lock_path(name):
    return (name.startswith("soc.cpus.cpumreg.") or
            name.startswith("soc.cpus.shared_ram.") or
            name.startswith("soc.ddr_ram_mux.u_dcache0.") or
            (".u_datapath.mem" in name and
             (name.startswith("soc.cpus.core0.") or
              name.startswith("soc.cpus.core1."))))


constrained = 0
for cname, cell in ctx.cells:
    if on_lock_path(str(cname)):
        try:
            ctx.constrainCellToRegion(cname, "lockcluster")
            constrained += 1
        except Exception:
            pass

print("floorplan_dual: region (%d,0)-(%d,%d), constrained %d lock-path cells"
      % (x0, x1, maxy, constrained))
