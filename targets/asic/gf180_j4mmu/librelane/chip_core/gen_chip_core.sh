#!/usr/bin/env bash
# Regenerate chip_core/chip_core.v: the hierarchical netlists (top/soc.v glue +
# the six child bodies) flattened into one `soc` module, with the vendor SRAMs
# kept as blackbox leaves. This is chip_top's only RTL input.
#
# `proc` is REQUIRED before `flatten`. Without it yosys warns
#   "Ignoring module soc because it contains processes (run 'proc' command first)"
# and write_verilog emits raw RTLIL processes; LibreLane's own yosys then
# hard-errors at Generate JSON Header with
#   "ERROR: Failed to get a constant init value for \aic_irq_gen.sync"
# The README recipe omitted `proc` until 2026-08-09, so following it produced a
# chip_core.v that only failed ~24s into the next chip_top run.
#
# Env:
#   REGEN_CHILDREN  non-empty -> rebuild the six child netlists first via
#                   run.sh OL_NETLIST_ONLY (seconds each, no LibreLane).
#   PDK_PIN         ciel pin holding the SRAM blackbox models.
set -euo pipefail
HERE="$(cd "$(dirname "$0")" && pwd)"
LB="$(dirname "$HERE")"                       # .../librelane
PIN="${PDK_PIN:-f6eeac7dad085ffcc829ccfd721f7b4ce39edcf7}"
SRAM="${CIEL_ROOT:-$HOME/.ciel}/ciel/gf180mcu/versions/$PIN/gf180mcuD/libs.ref/gf180mcu_fd_ip_sram/verilog"
if [ ! -d "$SRAM" ]; then
  echo "ERROR: SRAM blackbox models not found at $SRAM" >&2
  echo "       run: ciel enable --pdk-family gf180mcu $PIN" >&2
  exit 1
fi

# macro-dir/netlist-name for each child, in analyze order.
CHILD_MACROS=(cpus icache_2k dcache_2k smoke soc_cluster.devices qspi_flash)
CHILD_NETS=(cpus/cpus.v icache_2k/icache_adapter.v dcache_2k/dcache_adapter.v
            smoke/sdram_ctrl.v soc_cluster.devices/devices.v qspi_flash/qspi_flash_ctrl.v)

if [ -n "${REGEN_CHILDREN:-}" ]; then
  for m in "${CHILD_MACROS[@]}"; do
    echo "=== child netlist: $m ==="
    OL_NETLIST_ONLY=1 "$LB/run.sh" macro="$m"
  done
fi

MISSING=0
for c in "${CHILD_NETS[@]}"; do
  [ -f "$LB/$c" ] || { echo "ERROR: $LB/$c missing" >&2; MISSING=1; }
done
[ "$MISSING" -eq 0 ] || { echo "       rerun with REGEN_CHILDREN=1" >&2; exit 1; }

SRCS=("$LB/top/soc.v")
for c in "${CHILD_NETS[@]}"; do SRCS+=("$LB/$c"); done

yosys -q -p "
  read_verilog -lib $SRAM/gf180mcu_fd_ip_sram__sram64x8m8wm1__blackbox.v \
                    $SRAM/gf180mcu_fd_ip_sram__sram512x8m8wm1__blackbox.v;
  read_verilog ${SRCS[*]};
  hierarchy -top soc;
  proc;
  flatten;
  opt_clean;
  write_verilog -noattr $HERE/chip_core.v"

mods=$(grep -c '^module ' "$HERE/chip_core.v" || true)
srams=$(grep -cE 'sram(64|512)x8m8wm1 ' "$HERE/chip_core.v" || true)
inits=$(grep -c 'initial' "$HERE/chip_core.v" || true)
echo "chip_core.v: modules=$mods srams=$srams initial=$inits ($(wc -l < "$HERE/chip_core.v") lines)"

rc=0
[ "$mods"  -eq 1 ]  || { echo "ERROR: expected exactly 1 module (soc), got $mods" >&2; rc=1; }
[ "$srams" -eq 17 ] || { echo "ERROR: expected 17 SRAM instances (7 icache + 10 dcache), got $srams" >&2; rc=1; }
[ "$inits" -eq 0 ]  || { echo "ERROR: $inits 'initial' block(s) -- did 'proc' run before 'flatten'?" >&2; rc=1; }
[ "$rc" -eq 0 ] || exit 1
echo "OK: $HERE/chip_core.v"
