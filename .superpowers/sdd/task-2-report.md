# Task 2 Report: eth_tx device — regs + dual-clock buffer + SB_PLL40 (12->20 MHz)

## Files created
- `components/emac/eth_tx.vhd` — `entity eth_tx` (12 MHz CPU-bus device): SB_PLL40_CORE
  instance (12->20 MHz), dual-clock inferred frame buffer, 12 MHz register/bus
  process, CDC, and the `eth_tx_phy` instance.
- `components/emac/sb_pll40_core_sim.vhd` — behavioural sim-only `entity SB_PLL40_CORE`
  (free-runs 20 MHz on PLLOUTGLOBAL/PLLOUTCORE via `wait for 25 ns`; LOCK after 5 us).
- `components/emac/tests/eth_tx_tb.vhd` — self-checking GHDL testbench.

## PLL parameters
Command (run inside `ghcr.io/mountain-reverie/jcore-cpu-ci:latest`):
```
icepll -i 12 -o 20
```
Result: `F_PLLOUT achieved 19.875 MHz`, FEEDBACK SIMPLE, and:
- **DIVR = 0**  ("0000")
- **DIVF = 52** ("0110100")
- **DIVQ = 5**  ("101")
- **FILTER_RANGE = 1** ("001")

Passed as `std_logic_vector` generics to the `SB_PLL40_CORE` instance in `eth_tx.vhd`
(`FEEDBACK_PATH="SIMPLE"`, `PLLOUT_SELECT="GENCLK"`).

## Register map (decoded on db_i.a(11 downto 0))
- `0x800` write = TX_DATA: append 32-bit word at write pointer, auto-inc 4 bytes,
  big-endian (d(31:24) = byte 0 / lowest buffer address / first on the wire).
- `0x804` write bit0 = reset write pointer.
- `0x808` write = TX_LEN (byte count, latched to phy tx_len).
- `0x80C` write bit0 = TX_GO.
- `0x810` read bit0 = busy. `db_o.ack <= db_i.en`, `db_o.d <= rdata_r`.

## CDC approach
- **TX_GO clk -> clk_eth**: toggle bit `go_tgl` flips on each TX_GO; 2-FF synchronized
  into clk_eth; edge detect -> 1-cycle `tx_start` pulse. Level-safe regardless of clock
  ratio.
- **phy busy clk_eth -> clk**: 2-FF sync. `tx_done` pulses on the synchronized busy
  falling edge (busy is a held level for the whole frame, cannot be missed — unlike the
  1-cycle phy `done` pulse in the faster domain, which is not used for status).
- **busy readback race**: `go_pend` set at TX_GO, cleared once synced busy first seen
  high; `busy` readback = `busy_sync or go_pend`, so a poll right after GO never reads
  "done" spuriously.

## Test evidence
Analyze order: `cpu2j0_pkg.vhd`, `sb_pll40_core_sim.vhd`, `eth_tx_phy.vhd`,
`eth_tx.vhd`, `eth_tx_tb.vhd`. Flags `--std=93c -fexplicit -fsynopsys`; elaborate/run
add `--syn-binding` so `SB_PLL40_CORE` binds to the sim model. Run in the docker image
with the repo mounted at /work:
```
ghdl -a  --std=93c -fexplicit -fsynopsys  <files in order>
ghdl -e  --std=93c -fexplicit -fsynopsys --syn-binding eth_tx_tb
ghdl -r  --std=93c -fexplicit -fsynopsys --syn-binding eth_tx_tb --stop-time=5ms
```
Output:
```
/work/components/emac/tests/eth_tx_tb.vhd:168:5:@16208268500fs:(report note): eth_tx_tb PASSED
```
The tb writes the 8-byte frame (0x55 0xAB 0xCD 0x12 0x34 0x56 0x78 0x9A) as two
big-endian words over the bus, fires TX_GO, Manchester-decodes mdi_p/mdi_n (reusing the
eth_tx_phy_tb slot convention), asserts every decoded byte, polls busy(0x810) to 0, and
asserts tx_done pulsed. All checks use `severity failure`; "PASSED" only after they pass.

## NLP generic
Not added. eth_tx_phy.vhd is unmodified — the test completes in ~16.2 us, far before the
~16 ms NLP period, and the tb keys off the first post-GO line activity. Default NLP
period unchanged, no generic needed.

## Deviations / engineering calls
1. **Combinational (async) buffer read** instead of registered. The Task-1 `eth_tx_phy`
   samples `rd_data` in LOAD one cycle after driving `rd_addr` and consumes it the next
   cycle; its Task-1 ROM stub was asynchronous. A registered-read RAM delivers data one
   cycle too late (observed as byte1==byte0 on the first run). Read is therefore
   combinational: `rd_word <= mem(rd_addr/4)` + big-endian byte mux.
2. **tx_done via busy falling edge** rather than syncing the faster-domain `done` pulse.
3. **Not wired into a board filelist / `make check`.** Consistent with Task 1, which did
   not add `eth_tx_phy.vhd` to any `build.mk` or `targets/boards/icesugar/filelist.sh`.
   The device is verified standalone via the manual GHDL flow. The sim-only
   `sb_pll40_core_sim.vhd` carries the same "excluded from synth filelists" header as
   `sb_spram256ka_sim.vhd`; on future SoC integration it must be prepended only in
   `sim.sh` (like sb_mac16_sim / sb_spram256ka_sim), never in the shared `filelist.sh`.

## Self-review: risks / limitations
- **Async read vs iCE40 EBR**: iCE40 block RAM read ports are registered, so the
  combinational read will not map to EBR unmodified. Integration will need the phy to
  assert `rd_addr` a cycle earlier or a small read-ahead pipeline. Flagged; does not
  affect this functional milestone.
- **PLL model free-runs** (edges at 25 ns + k·50 ns), phase-fixed. The tb reconstructs
  slot timing from the first line transition only; it makes no assumption about
  REFERENCECLK/PLL phase, so the CDC is genuinely exercised.
- **Buffer depth** 512 words (2 KiB) — full 1518-byte frame fits; TX_LEN/rd_addr 12-bit.
- The `NUMERIC_STD.TO_INTEGER: metavalue` warning at @0 ms is uninitialized `rd_addr`
  before the phy drives it — harmless, pre-reset only.

## Fix

Three correctness bugs (flagged by the self-review above / found in follow-up review)
fixed on top of the original Task 2 work, all on `feat/icesugar-eth-tx`.

### Fix 1 (Critical): `eth_tx_phy.vhd` buffer read model + Manchester gap

**Was wrong:** `eth_tx_phy` set `rd_addr <= byte_idx` combinationally and sampled
`rd_data` one cycle later in a `LOAD` state, i.e. it assumed a **combinational**
buffer read. A real iCE40 `SB_RAM40` read port is **registered** (data valid the
cycle *after* the address is presented), so on real hardware every byte would be
fetched one cycle early — frame corruption. Separately, the per-byte `LOAD` state
held the differential output for an extra cycle between bytes, stretching the last
half-bit of every byte by one clk_eth cycle — a Manchester timing violation (real
10BASE-T requires every half-bit to be exactly one bit period, with no holds).

**What changed:** `eth_tx_phy` is now a continuous, gapless Manchester serializer
with byte **prefetch**. There is no `LOAD` state during the frame anymore. While
shifting out the current byte (16 clk_eth cycles: 2 half-bits x 8 bits), `rd_addr`
is held at `byte_idx+1` for the whole byte (15 cycles of margin, `nxt_byte` is
continuously latched from the registered `rd_data`), and at the byte boundary
`cur_byte <= nxt_byte` rolls straight into the next byte on the very same edge that
`bit_idx`/`byte_idx` wrap — no gap. Byte 0 is primed by a 2-cycle `PREFETCH` state
before the frame's first Manchester transition (start-of-frame pipeline-fill
latency, not a mid-frame gap). After the last byte: `TPIDL` then idle, unchanged.
Port names/types are unchanged.

`eth_tx.vhd`'s frame buffer read port is now **registered** (a clocked process,
`ram_rd_proc`, latching `rd_word_r`/`rd_bsel_r` on `clk_eth`) instead of the old
combinational `mem(...)` read, so simulation matches real EBR timing and yosys
`synth_ice40` has a chance at inferring `SB_RAM40` (see re-verification below —
it does not, for other reasons).

### Fix 2 (Important): `db_o.ack` must be combinational

**Was wrong:** `eth_tx.vhd` registered `db_o.ack` (`ack_r`, set from
`reg_proc`), adding an extra bus-turnaround cycle inconsistent with every other
device in this codebase.

**What changed:** `ack_r` removed; `db_o.ack <= db_i.en;` added as a top-level
concurrent assignment, mirroring `components/uartlite/uart.vhd` /
`components/misc/pio.vhd` / `components/misc/spi2.vhd` / `components/misc/gpio2.vhd`.
`eth_tx_tb.vhd`'s `bus_write`/`bus_read` procedures now `wait until db_o.ack = '1'`
instead of assuming a fixed cycle count, so ack timing is actually exercised
(`bus_read` still waits one further `clk` edge after the ack for `rdata_r`, which
remains a registered readback — that part of the contract is unchanged).

### Fix 3 (Important): `tx_len` CDC bug

**Was wrong:** `tx_len_r` (12-bit, `clk` domain) was wired straight into
`eth_tx_phy`'s `tx_len` port, which lives in the `clk_eth` domain — an
unsynchronized multi-bit CDC.

**What changed:** `tx_len_r` is now latched into a `clk_eth`-domain register
(`tx_len_eth`) at the `tx_start` pulse (`eth_proc`), with a one-line comment
documenting the assumption this relies on: software writes `TX_LEN` before
`TX_GO`, and the GO toggle-sync already adds a couple of `clk_eth` cycles of
margin, so `tx_len_r` is stable by the time `tx_start` fires. `eth_tx_phy` now
takes `tx_len_eth`, not `tx_len_r`.

Fixing this exposed a second, self-inflicted race: `eth_tx_phy` was fed `tx_start`
directly, and it samples `tx_len` at the *same* edge `tx_start` is sampled — but
`tx_len_eth` isn't valid until the cycle *after* that latching edge, so the phy
saw `tx_len=0` in the same cycle it saw `tx_start='1'` and silently ignored the
whole frame (`tx_len_eth` = 0 fails the `tx_len /= 0` idle-exit guard). Fixed by
adding `tx_start_d` (`tx_start` delayed one more `clk_eth` cycle) and feeding
*that* to the phy's `tx_start` port, so `tx_len_eth` is already stable when the
phy consumes it. This added exactly one `clk_eth` cycle of start-of-frame
latency; `eth_tx_tb.vhd`'s decode timing is unaffected because it measures from
the first observed line transition (`t_first`), not from a fixed offset off GO.

### Testbench fixes (found while re-verifying)

- `eth_tx_phy_tb.vhd`'s byte-source ROM stub changed from a combinational
  `process(rd_addr)` lookup to a registered `process(clk_eth)` lookup, matching
  the real EBR timing contract.
- `eth_tx_phy_tb.vhd`'s Manchester decode slot formula updated for the new
  16-cycles/byte (was 17, the old `LOAD` state's extra cycle) FSM timing, and a
  bug in the mid-slot sample-time formula (`+1) + 0.5` where it should have been
  `+ 0.5` — an off-by-one half-cycle that, combined with the cycle-count change,
  was initially caught by a full byte-shuffle mismatch on bytes 1..7 during
  re-verification (root-caused via a `--vcd` waveform dump, see below).
- `eth_tx_phy_tb.vhd` gained a `busy_cycles` counter and a final assertion
  `busy_cycles = 2 + 16*NBYTES + 1`, directly verifying the gapless/no-stray-cycle
  property (any reintroduced hold state inflates this count).
- `eth_tx_tb.vhd`'s decode slot formula updated from `17*i` to `16*i` per byte
  (same FSM cycle-count change); this formula was already correctly derived
  relative to `t_first`, no off-by-one there.

## Re-verification (docker GHDL)

All runs in `ghcr.io/mountain-reverie/jcore-cpu-ci:latest`, repo mounted at
`/work`, mirroring the Task 2 invocation pattern (`--std=93c -fexplicit
-fsynopsys`, `--syn-binding` for `eth_tx_tb` so `SB_PLL40_CORE` binds to the
behavioural sim model).

### eth_tx_phy_tb

```
ghdl -a --std=93c -fexplicit -fsynopsys components/emac/eth_tx_phy.vhd
ghdl -e --std=93c -fexplicit -fsynopsys eth_tx_phy_tb   # (after -a'ing the tb)
ghdl -a --std=93c -fexplicit -fsynopsys components/emac/tests/eth_tx_phy_tb.vhd
ghdl -e --std=93c -fexplicit -fsynopsys eth_tx_phy_tb
ghdl -r --std=93c -fexplicit -fsynopsys eth_tx_phy_tb --stop-time=1ms
```
Output:
```
/work/components/emac/tests/eth_tx_phy_tb.vhd:167:5:@7026ns:(report note): eth_tx_phy_tb PASSED
```
This includes the new gapless assertion (`busy_cycles = 2 + 16*8 + 1 = 131`) passing
alongside the per-byte Manchester decode checks and the done/busy checks.

### eth_tx_tb

```
ghdl -a --std=93c -fexplicit -fsynopsys components/cpu/cpu2j0_pkg.vhd \
                                         components/emac/sb_pll40_core_sim.vhd \
                                         components/emac/eth_tx_phy.vhd \
                                         components/emac/eth_tx.vhd \
                                         components/emac/tests/eth_tx_tb.vhd
ghdl -e --std=93c -fexplicit -fsynopsys --syn-binding eth_tx_tb
ghdl -r --std=93c -fexplicit -fsynopsys --syn-binding eth_tx_tb --stop-time=5ms
```
Output:
```
/work/components/emac/tests/eth_tx_tb.vhd:174:5:@15958269500fs:(report note): eth_tx_tb PASSED
```
This exercises the combinational `db_o.ack` via `wait until db_o.ack = '1'` in both
`bus_write` and `bus_read`, the registered buffer read, the fixed `tx_len` CDC latch
(+ the `tx_start_d` fix), and Manchester-decodes/asserts the full 8-byte frame plus
the busy-poll / `tx_done` checks — all with `severity failure`.

### yosys `synth_ice40` smoke test (optional) — ATTEMPTED, SB_RAM40 **not** inferred

```
yosys -m ghdl -p "ghdl --std=93c -fsynopsys components/cpu/cpu2j0_pkg.vhd \
                                             components/emac/eth_tx_phy.vhd \
                                             components/emac/eth_tx.vhd -e eth_tx; \
                   blackbox SB_PLL40_CORE; synth_ice40 -top eth_tx"
```
Ran cleanly (`SB_PLL40_CORE` treated as an unbound blackbox, 0 CHECK problems), but
the frame buffer did **not** map to `SB_RAM40` — it fell back to distributed
LUT/FF RAM:
```
Number of cells:              31939
  SB_CARRY                       45
  SB_DFF                         34
  SB_DFFE                     16400
  SB_DFFESR                      74
  SB_DFFSR                       11
  SB_LUT4                     15373
  SB_PLL40_CORE                   1
```
`MEMORY_COLLECT` ran but `memory_bram` evidently declined to map it to
`SB_RAM40_4K`, most likely because the buffer's write port (32-bit) and read port
(8-bit) have mismatched geometry, which `SB_RAM40` requires to line up (power-of-2
ratio with matching total width) more strictly than this asymmetric byte/word
split provides. This is a real synthesis-quality concern for future integration
(1518-byte frame buffer as flops is far too expensive for an up5k), but it is
**not** a functional/correctness bug of the fixes in this task, is orthogonal to
gaplessness/CDC correctness, and reproducing/fixing it (e.g. splitting the buffer
into 4 byte-wide RAMs, one selected per `rd_bsel`, to get symmetric geometry) is
flagged as a follow-up rather than blocking this task.
