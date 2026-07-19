# OpenRAM / GF180 SRAM macro spike (Task 2)

This directory holds the **reproducible environment + config** for the
OpenRAM/GF180 feasibility spike. See `../DECISION.md` for the spike's
conclusions and go/no-go for phase 2c. Generated outputs
(`.gds/.lib/.lef/.v/.sp/.log`, the `out/` dir) are **gitignored** — they are
large, SPICE-derived, and reproducible from the commands below.

## Why this exists

Plan 2 (GF180 J4+MMU ASIC target) needs to know what port configuration
OpenRAM can actually generate for GF180 before committing to generating the
full cache SRAM macro set (phase 2c). This spike stands up OpenRAM against
the pip-distributed `gf180mcu` tech tree, generates ONE small macro, and
records what it produced.

## Environment (exact, reproducible, no sudo / no repo-committed binaries)

Everything below was installed into a scratch venv and a scratch directory
**outside the repo** — nothing here requires root and nothing is committed
to the repo except this config + README + DECISION.md.

1. **Python venv + OpenRAM (pip)**

   ```bash
   python3 -m venv /path/to/scratch/openram_venv
   source /path/to/scratch/openram_venv/bin/activate
   pip install openram==1.2.48
   ```

   OpenRAM 1.2.48 ships its technology trees inside the wheel at
   `<venv>/lib/python3.10/site-packages/openram/technology/{sky130,gf180mcu}`.
   No separate `OPENRAM_TECH` checkout is required — `gf180mcu` is bundled.
   `OPENRAM_HOME` is auto-derived by `openram/__init__.py` to
   `.../openram/compiler` if not set.

2. **ngspice (SPICE engine OpenRAM shells out to for characterization/sim)**

   Not installed on this box and no passwordless sudo was available, so it
   was obtained without root via `apt-get download` (downloads the .deb
   without installing) + manual extraction:

   ```bash
   mkdir -p /path/to/scratch/ngspice_local && cd /path/to/scratch/ngspice_local
   apt-get download ngspice            # ngspice 36+ds-1ubuntu0.1 (jammy-updates)
   dpkg-deb -x ngspice_36+ds-1ubuntu0.1_amd64.deb extracted
   export PATH="$PWD/extracted/usr/bin:$PATH"
   ngspice --version                   # confirms: ngspice-36
   ```

   `ldd` on the extracted binary showed no missing shared libraries, so no
   further system packages were needed for ngspice itself.

3. **klayout (GDS backend)** — the standalone `klayout` binary .deb is a thin
   wrapper around a large shared-library tree that pulls in system Qt/Ruby
   libs not present on this box; rather than chase that dependency graph,
   the **pip `klayout` Python package** (klayout 0.30.9, a self-contained
   standalone build of the KLayout Python API) was used instead — OpenRAM's
   GDS writer path in the version tested works from Python without the
   klayout GUI/CLI binary:

   ```bash
   pip install klayout   # 0.30.9, self-contained, no system Qt/Ruby needed
   ```

4. **Confirm the tool runs and can see the GF180 tech**

   ```bash
   source /path/to/scratch/openram_venv/bin/activate
   export PATH=/path/to/scratch/ngspice_local/extracted/usr/bin:$PATH
   cd /path/to/scratch/openram_venv/lib/python3.10/site-packages/openram
   python3 sram_compiler.py --help
   ```

   `sram_compiler.py` must be launched from inside the installed
   `openram/` package directory (it does `from common import *` with a
   relative-path assumption) — running it as `python3 -m openram.sram_compiler`
   fails with `ModuleNotFoundError: No module named 'common'`.

5. **Point OpenRAM at the GF180 open_pdks tech files (`PDK_ROOT`)**

   OpenRAM's `gf180mcu` tech module (`.../openram/technology/gf180mcu/__init__.py`)
   requires `$PDK_ROOT/gf180mcuD/libs.tech/{magic,netgen,ngspice}` — it does
   NOT ship these itself (only the bitcell/decoder cell views + layer map
   are bundled in the pip package, under `technology/gf180mcu/{mag_lib,
   gds_lib,sp_lib,tech}`). The GF180 PDK fetched via `ciel` for Task 1
   already has exactly this layout:

   ```bash
   export PDK_ROOT=~/.ciel/ciel/gf180mcu/versions/f6eeac7dad085ffcc829ccfd721f7b4ce39edcf7
   ls $PDK_ROOT/gf180mcuD/libs.tech/ngspice/sm141064.ngspice   # must exist
   ls $PDK_ROOT/gf180mcuD/libs.tech/magic/gf180mcuD.magicrc    # must exist
   ls $PDK_ROOT/gf180mcuD/libs.tech/netgen/setup.tcl           # must exist
   ```

   Without `PDK_ROOT` set, `sram_compiler.py` fails immediately with
   `SystemError: Unable to find open_pdks tech file. Set PDK_ROOT.`

## Outcome

**Generation did not succeed.** After fixing the environment (below), the
single-port 512x32 GF180 macro fails at netlist-construction time with a
missing-standard-cell error (`gf180mcu/sp_lib/` only ships a bitcell and one
NAND2 gate — no `dff`, no inverter, no periphery cells). See
`../DECISION.md` §2 for the full evidence and root cause, and §5 for the
exact repro of the failure. This is documented as the spike's outcome, not
worked around.

## Config in this directory

- `ram_32x512_1rw.py` — single-port (1RW), 512 words x 32 bits, matching the
  `RAM_32x1x512` geometry from `lib/memory_tech_lib/ram_2rw.vhd`
  (`mem_layout_t`), chosen per the task brief as the smallest 2rw geometry to
  minimize spike generation time. **It requests `num_rw_ports = 1`, not 2**,
  because GF180's OpenRAM tech tree only registers a single-port bitcell —
  see `../DECISION.md` for why a true/near dual-port config could not even be
  requested.

## Reproduction: generate the macro

```bash
source /path/to/scratch/openram_venv/bin/activate
export PATH=/path/to/scratch/ngspice_local/extracted/usr/bin:$PATH
mkdir -p /path/to/scratch/openram_run/out
sed "s#OUTPUT_PATH_PLACEHOLDER#/path/to/scratch/openram_run/out#" \
  lib/memory_tech_lib/tech/gf180/openram/ram_32x512_1rw.py \
  > /path/to/scratch/openram_run/cfg.py

cd /path/to/scratch/openram_venv/lib/python3.10/site-packages/openram
nohup python3 sram_compiler.py -n -v /path/to/scratch/openram_run/cfg.py \
  > /path/to/scratch/openram_run/log.txt 2>&1 &
# `-n` disables LVS/DRC (magic/netgen not installed; out of scope for a
# port-config feasibility spike). Run in the background/poll — SPICE-backed
# generation is slow.
```

Outputs land in `/path/to/scratch/openram_run/out/` as
`sram_32x512_1rw_gf180.{gds,lef,lib,v,sp}` (not committed — see
`.gitignore`).
