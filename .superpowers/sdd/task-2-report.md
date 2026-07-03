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
