# DECISION: OpenRAM / GF180 2RW SRAM macro feasibility (Task 2 spike)

Status: **BLOCKED** — OpenRAM (pip `openram==1.2.48`) cannot generate a
complete SRAM macro of ANY port configuration for GF180 with the tech tree
it ships. This is a legitimate spike outcome per the task brief ("if you
hit a hard blocker ... that is a LEGITIMATE spike outcome"). See §2 for the
two independent, compounding blockers found and the evidence for each, and
§4 for the go/no-go this implies. See `openram/README.md` for the
environment and exact reproduction commands (including how to reproduce
the failure).

## 1. Real port requirement of the `ram_2rw` cache instances (from RTL)

Read directly from `components/cpu/cache/dcache_ram.vhd` and
`components/cpu/cache/icache_ram.vhd` (not modified by this spike).

- **dcache data RAM** (`dcache_ram.vhd:79-148`, `ram_2rw` x2, one per byte
  lane): comment at line 63 confirms port0 (CPU load/store, clk125) and
  port1 (line-fill, clk200) writes are mutually exclusive per cycle (the
  blocking FSM forces `wr0='0'` during a refill); the only real concurrency
  is a port0 **read** (CPU load, hit-under-miss) simultaneous with a port1
  **write** (refill) at a different index. Both cache_clkmode variants
  (`CACHE_SAME_CLOCK` true/false) always tie `dr1 => open` — **port1's read
  output is never connected to anything**, i.e. port1 is write-only in the
  RTL as instantiated. So dcache data = **1RW (port0) + 1W (port1)**, not
  true 2RW.
- **icache data RAM** (`icache_ram.vhd:45-68`, `ram_2rw` x2): port0 has
  `wr0` tied to the constant `'0'` — it is **always** a pure read port (no
  CPU store path into the icache), and port1 is again write-only
  (`dr1 => open`, refill only). So icache data = **1R (port0) + 1W
  (port1)** — an even simpler case than dcache (port0 never writes at all).
- **Tag RAMs** (both dcache and icache): tags are NOT instantiated as
  `ram_2rw` at all. `dcache_ram.vhd:26-56` instantiates **two separate
  `ram_1rw` single-port macros** (`tag0`, `tag1`, one per cache way) sharing
  the same address/control signals (`ra.ten0`/`ra.twr0`) but with
  independent `dr0`/`dr1` *signal names* (`tag_dr0`, `tag_dr1`) that are
  just per-way single-port RAM outputs — not a dual-port RAM's two ports.
  Same pattern in `icache_ram.vhd:25-39` (single `tag : ram_1rw`).
  **Conclusion: the tag storage never needs a `ram_2rw`/dual-port macro at
  all** — it is two (dcache) or one (icache) plain single-port (1RW) SRAM
  instances. This removes tags entirely from the "does OpenRAM do dual-port"
  question; tags only need whatever OpenRAM already produces for `ram_1rw`
  (a plain 1RW macro, which is exactly the mainline/best-supported OpenRAM
  case for every technology, including GF180).

**Net requirement across all four `ram_2rw` data-RAM instances (dcache x2,
icache x2):** port1 read (`dr1`) is unused in every instance. The cache's
real need is **1RW+1W** (dcache) / **1R+1W** (icache) — never true 2RW
(2 independent read+write ports). This matches the task brief's framing
exactly and is the binding requirement handed to OpenRAM below.

## 2. What OpenRAM can actually produce for GF180

**Finding: OpenRAM (pip `openram==1.2.48`) cannot generate a complete SRAM
macro for GF180 at all — single-port or multi-port.** Two independent,
compounding gaps were found in the shipped `gf180mcu` tech tree:

### 2a. No dual-port bitcell (blocks true 2RW / 1RW+1W / 1R+1W)

- `gf180mcu/tech/tech.py` registers exactly one bitcell module:
  ```
  tech_modules["bitcell_1port"] = "gf180_bitcell"
  ```
  There is **no** `tech_modules["bitcell_2port"]` (nor
  `replica_bitcell_2port` / `dummy_bitcell_2port`) entry anywhere in the
  `gf180mcu` tech tree.
- `gf180mcu/custom/gf180_bitcell.py` hard-codes a single-port cell:
  ```python
  cell_name = "cell1rw"
  super().__init__(name, cell_name=cell_name, prop=props.bitcell_1port)
  ```
- By contrast, `sky130/tech/tech.py` registers the full 2-port family:
  `tech_modules["bitcell_2port"] = "bitcell_2port"`,
  `replica_bitcell_2port`, `dummy_bitcell_2port`, plus 2-port row/col cap
  variants — i.e. OpenRAM's dual-port SRAM compiler path exists and is
  technology-generic in the compiler core, but **GF180 was never given a
  dual-port bitcell layout/spice/lvs view** to plug into it. Known
  upstream OpenRAM/GF180-PDK gap, not a bug in this spike's config.
- Consequence: `num_rw_ports`/`num_r_ports`/`num_w_ports` are generic knobs
  in `openram.compiler.options` (defaults `1/0/0`), but for
  `tech_name = "gf180mcu"` any request needing `bitcell_2port` fails at
  tech-module lookup — there's nothing to substitute; it doesn't downgrade
  the port count silently, the module is simply absent.

Because of (2a) alone, only `num_rw_ports = 1` (plain single-port) was
requested for the one macro this spike attempted to generate — see
`openram/ram_32x512_1rw.py` (512 words x 32 bits, matching `RAM_32x1x512`
from `lib/memory_tech_lib/ram_2rw.vhd`, `TT` corner only,
`check_lvsdrc = False`, `analytical_delay = True`).

### 2b. Even single-port generation fails: the gf180mcu SPICE cell library is a stub

Running the single-port config (`sram_compiler.py -n -v ram_32x512_1rw.py`,
full repro in `openram/README.md`) fails with:

```
ERROR: file design.py: line 44: Custom cell pin names do not match spice file:
['D', 'Q', 'clk', 'vdd', 'gnd'] vs []
AssertionError
```

Root cause: `openram/technology/gf180mcu/sp_lib/` — the directory OpenRAM
reads hand-written SPICE subcircuits from for its "custom" (non-generated)
standard cells — contains **only two files**:

```
sp_lib/cell1rw.sp                       (the 1-port bitcell)
sp_lib/gf180mcu_3v3__nand2_1_dec.sp     (one 2-input NAND decoder gate)
```

There is no `dff.sp` (or any flip-flop), no inverter, no sense amp, no
write driver, no precharge, no column mux cell — none of the periphery
logic every OpenRAM SRAM (regardless of port count) needs to build its
control/data path. `sram_1bank.add_modules()` fails on the very first
`factory.create(module_type="dff")` call because
`cell_properties.dff.port_names` (`['D','Q','clk','vdd','gnd']`, from
`gf180mcu/tech/tech.py`'s `spice[...]`/`parameter[...]` DFF timing
constants) has no matching SPICE subckt to bind to — `self.pins` comes
back empty.

For scale: the entire `gf180mcu` tech directory is **18 files**
(bitcell layout/spice/mag/gds + one decoder gate + tech/layer-map files);
`sky130`'s tech directory (a technology OpenRAM fully supports, including
dual-port) is **42 files** and includes a much larger hand-built/generated
cell set. **The GF180 tech port in OpenRAM 1.2.48 is a partial/WIP stub —
essentially just the bitcell array physical view — not a complete,
usable SRAM-compiler backend**, independent of the port-count question in
§2a.

**Neither blocker is a config mistake in this spike** — §2a was confirmed
by inspecting `tech_modules` registration (a static, deliberate omission),
and §2b was confirmed by a minimal Python repro that isolates the failure
to a missing `.sp` file, not a bad generic (`options.py`) setting (see
`openram/README.md` "Repro of the failure" for the exact isolating script).

## 3. Measured area

**No macro was generated — no area number exists.** The single-port
512x32 GF180 attempt (the smallest, simplest possible macro, chosen per
the task brief specifically to minimize spike time) fails before layout
begins, at netlist-construction time (`sram_1bank.create_netlist()` →
`add_modules()` → first `dff` instantiation), because of the missing
`dff.sp` (§2b). It never reaches placement, routing, GDS emission, or
`.lib` characterization, so there is no `.gds`/`.lib` to measure and no
empirically-grounded number to extrapolate from for the full cache macro
set (dcache data x2 @ 2048x16b, icache data x2 @ 2048x16b, dcache tags x2
@ 256x16b, icache tag x1 @ 256x16b — geometries per the `ram_1rw`/`ram_2rw`
generic maps in `dcache_ram.vhd`/`icache_ram.vhd`).

Any area estimate at this point would be a guess dressed up as data; this
DECISION.md does not fabricate one. A real area number requires either
completing the GF180 standard-cell library gap (§2b) in OpenRAM, or
switching to a different SRAM generation path entirely (§4).

## 4. Go/no-go for phase 2c

**NO-GO for 2c as scoped ("generate the macro set with OpenRAM/GF180")** —
OpenRAM's pip-shipped `gf180mcu` tech cannot generate a complete SRAM macro
of any port configuration today (§2a AND §2b). This is stronger than "no
dual-port": even the tag macros' plain single-port `ram_1rw` — which would
otherwise be an easy GO, since tags were never `ram_2rw` in the first place
(§1) — cannot be generated either, because §2b (missing `dff`/periphery
cells) blocks single-port generation too.

**Before 2c can start, one of these must happen** (all out of scope for
this spike, which is a knowledge/decision gate, not an implementation
task):

1. **Fix/complete the OpenRAM GF180 tech tree.** Add the missing
   `sp_lib` cells (dff, inverter, sense amp, write driver, precharge,
   column mux — whatever `sram_1bank`/`bitcell_array` periphery needs) and
   a dual-port bitcell view, either by hand or by pulling a more complete
   community fork/branch of OpenRAM's `gf180mcu` support if one exists
   upstream (not checked in this spike — worth a scan of the OpenRAM GitHub
   issue tracker/branches before committing to writing it from scratch).
   This is a nontrivial standard-cell + bitcell layout effort, not a config
   change.
2. **Use GF180's own vendor hard SRAM IP instead of OpenRAM.** The ciel
   GF180 PDK checkout already used for Task 1
   (`~/.ciel/ciel/gf180mcu/versions/f6eeac7dad085ffcc829ccfd721f7b4ce39edcf7/gf180mcu{A,B,C,D}/libs.ref/gf180mcu_fd_ip_sram/`)
   ships four **pre-hardened, ready-to-use** GlobalFoundries SRAM macros —
   real `.gds`, `.lef`, `.lib` (multiple corners), `.spice`/`.cdl`, and
   Verilog blackbox models already present on disk, no generation step
   needed at all:
   `sram64x8m8wm1`, `sram128x8m8wm1`, `sram256x8m8wm1`, `sram512x8m8wm1`
   (all `N`-words x **8-bit** word). Inspecting the Verilog port list
   (`CLK, CEN, GWEN, WEN, ...`) confirms these are **single-port** (one
   clock, one chip-enable) macros — so this path directly and immediately
   satisfies the tag macros' need (§1: `ram_1rw`, plain single-port) with
   zero tool risk, but does **not** solve the data macros' 1RW+1W/1R+1W
   need any more than OpenRAM would (still single-port; still needs the
   §4.3 arbitration fallback for data RAMs, OR a different vendor IP
   offering — worth checking whether GF180 also ships a genuinely
   dual-port SRAM IP under a name not yet located in this spike).
   **This is the most promising, lowest-risk next step for the *tag*
   macros** and should be the first thing the follow-up plan tries — it
   sidesteps both OpenRAM blockers (§2a, §2b) entirely for that half of the
   macro set, at the cost of fixed word width (8 bits, vs. the RTL's 16-bit
   tag words — would need two 8-bit macros ganged per tag way, or a byte-
   interleave, to build the 256x16b tag geometry) and fixed depths (64/128/
   256/512 — 256 exactly matches the tag depth from `ADDR_WIDTH => 8` in
   `dcache_ram.vhd`/`icache_ram.vhd`).
3. If neither (1) nor (2) pans out, the RTL-side fallback from the
   original plan still stands as the last resort: the cache's data-RAM
   real need (§1, 1RW+1W / 1R+1W) would have to be built from single-port
   macros + SoC-side arbitration (replicated-write dual single-port banks,
   or a bank-interleave/arbiter with a new stall path) — but this can only
   be evaluated once single-port GF180 generation itself works (via (1) or
   (2)), since right now **not even one single-port macro can be built**.

**Tag-macro finding stands regardless of the blocker above**: tags
(`ram_1rw`, dcache x2 @ 256x16b + icache x1 @ 256x16b) never needed
dual-port silicon (§1) — so once *either* (1) or (2) above unblocks
single-port GF180 generation, tags are the easy/first macro to validate
end-to-end, exactly as this spike attempted.

## 5. Reproduction

See `openram/README.md` for the full environment setup (pip `openram==1.2.48`,
`ngspice` extracted from the Ubuntu jammy .deb without root, `klayout` pip
package, `PDK_ROOT` pointed at the ciel GF180 PDK checkout
`~/.ciel/ciel/gf180mcu/versions/f6eeac7dad085ffcc829ccfd721f7b4ce39edcf7`)
and the exact generation command:

```bash
export PDK_ROOT=~/.ciel/ciel/gf180mcu/versions/f6eeac7dad085ffcc829ccfd721f7b4ce39edcf7
source <scratch>/openram_venv/bin/activate
export PATH=<scratch>/ngspice_local/extracted/usr/bin:$PATH
cd <scratch>/openram_venv/lib/python3.10/site-packages/openram
python3 sram_compiler.py -n -v lib/memory_tech_lib/tech/gf180/openram/ram_32x512_1rw.py
```

This reproduces the §2b failure (`AssertionError` at
`compiler/base/design.py:44`, `dff` pins `[]` vs. expected
`['D','Q','clk','vdd','gnd']`) end to end.

The critical, non-obvious piece to even GET to that failure: **`PDK_ROOT`
must point at a directory containing a
`gf180mcuD/libs.tech/{magic,netgen,ngspice}` tree** (that's what
`openram/technology/gf180mcu/__init__.py` looks for) — the ciel-managed
GF180 PDK checkout already has exactly this layout, so no separate
"OpenRAM GF180 tech" download was needed beyond what ciel had already
fetched for Task 1. Without `PDK_ROOT` set, the run fails earlier with
`SystemError: Unable to find open_pdks tech file. Set PDK_ROOT.`

A minimal isolating repro for §2b specifically (skips the full compiler,
shows the missing-cell failure directly):

```bash
cd <scratch>/openram_venv/lib/python3.10/site-packages/openram
python3 - <<'EOF'
import sys; sys.path.insert(0, '.')
import common; common.make_openram_package()
import openram
openram.init_openram(config_file="<path-to>/ram_32x512_1rw.py")
from openram.compiler.sram_factory import factory
factory.create(module_type="dff")   # AssertionError: pins [] vs ['D','Q','clk','vdd','gnd']
EOF
ls <venv>/lib/python3.10/site-packages/openram/technology/gf180mcu/sp_lib/
# -> only cell1rw.sp and gf180mcu_3v3__nand2_1_dec.sp: no dff, no other gates
```
