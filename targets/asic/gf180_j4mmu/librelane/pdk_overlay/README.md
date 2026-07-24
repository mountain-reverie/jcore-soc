# GF180 LibreLane PDK compat overlay

Working (patched) `libs.tech/librelane` Tcl for **ciel pin
f6eeac7dad085ffcc829ccfd721f7b4ce39edcf7 (gf180mcuC) + LibreLane 2.4.2**.
`apply.sh` overlays it onto a freshly `ciel enable`d PDK so the P&R flow is
reproducible off the dev box (CI, self-hosted runner). See apply.sh header for
the two bug classes patched. Pin-specific; re-capture if the pin changes.
GF180 PDK is Apache-2.0 (redistribution of modified config.tcl permitted w/ attribution).
