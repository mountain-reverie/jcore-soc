# UART LC-reduction trim: `rx_enable` generic

## Goal
The iCESugar (iCE40 UP5K) build was 38 ICESTORM_LC over budget (5318/5280) after
adding Ethernet RX. Reclaim LCs by dropping the shared `uartlite` UART's unused
receive + interrupt logic on iCESugar, whose boot only writes the UART.

## Result
**FITS.** iCESugar sim still PASSES; ICESTORM_LC **5318 → 5190** (saved 128 LC),
well under the 5280 budget (90 LC of headroom). `rx_enable=false` alone was
enough — no second trim needed.

## Generic added
- `rx_enable : boolean := true` — added to:
  - the `uartlite` entity (`components/uartlite/uart.vhm`, canonical source, and
    the generated `components/uartlite/uart.vhd`)
  - the `uartlite` component decl in `components/uartlite/uart_pkg.vhd`
  - the `uartlitedb` wrapper entity + its `uart` instance generic map
    (`components/uartlite/uartlitedb.vhd`)

A separate `irq_enable` was **not** needed: on iCESugar the UART irq is already
`open` in devices.vhd, and all irq assertions in the UART live inside the RX
datapath (RX-non-empty / RX-timeout). The only non-RX interrupt is the TX-empty
interrupt, which is gated by `this.ien` (software-enabled via the control reg)
and is harmless/dead when software never enables it. Guarding RX therefore
removes every RX-sourced interrupt path; no extra generic was justified.

## What is guarded behind `rx_enable`
When `rx_enable = false`, the following are not synthesized (VHDL `if rx_enable
then ...` static-elaboration guards, so false-branch logic is optimized away):
1. **RX baud/shift/state machine** — the entire "receiver" block (majority-vote
   sampling, end-of-frame, bit shift-in, RX state machine) at the 16x baud tick.
2. **RX FIFO push** — `rfc`/`rxfw` writes into `rxf` are only driven from that
   guarded block, so the RX FIFO write port collapses.
3. **RX/timeout interrupt logic** — both the `intcfg=1` (RX-non-empty) and
   `intcfg=0` (RX-timeout via `itimeout`/`rxint`) interrupt assertions.
4. **RX register reads** — DATA read returns 0 (no FIFO pop); status read returns
   the RX-related bits (ferr, ovr, rx_full, rx_valid) as constant 0. TX-related
   status bits (ien, tx_full, tx_empty) are unchanged.
5. **RX metastable input buffer** — `this.rx.a := this.rx.a(0) & rx;` is guarded,
   so the `rx` input port has no fanout and its sync flops drop out.

The RX FIFO storage array (`rxf`) itself is instantiated with `rx_fifo_len=1` on
iCESugar (already minimized) and, with no write port and reads folded to 0,
optimizes away.

## TX / bus interface unchanged
The transmitter datapath + state machine, the `dds` baud generator, the CPU-side
DATA/CTRL write interface, the TX FIFO, and the `ack` handshake are entirely
outside the guards and are byte-identical regardless of `rx_enable`. This is why
the sim's UART-TX banner (which several checks read) still passes.

## Shared-code safety (default true unchanged)
- `rx_enable` defaults to `true` in the entity, the component decl, and the
  `uartlitedb` wrapper. With the default, the guards `if rx_enable then` wrap the
  original code verbatim (only re-indented), so the elaborated netlist is
  identical to before. Turtle / mimas / microboard, which use RX and don't set
  the generic, are functionally unchanged.
- The common device class `uartlite` (`common_device_classes.yaml`) passes any
  generic through unchanged; a board that doesn't set `rx_enable` gets the entity
  default (true). Only iCESugar's `design.yaml` sets `rx_enable: false`.
- Verified the generated `targets/boards/icesugar/devices.vhd` binds
  `rx_enable => FALSE` on `uart0`; the other boards' devices.vhd are untouched.

## Evidence
### iCESugar sim (`./targets/boards/icesugar/sim.sh`) — PASS
```
pad_ring elaborated OK
icesugar_top_tb: ETH FRAME OK (decoded+CRC-verified 28-byte frame matches)
icesugar_top_tb: ARP REPLY OK (decoded+CRC-verified 54-byte ARP reply matches)
icesugar_top_tb PASSED: FROM SPRAM + SPRAM MEMTEST OK + ETH FRAME OK + ARP REPLY OK
ghdl:info: simulation stopped by --stop-time @210ms
```
(exit code 0)

### iCESugar synth (`./targets/boards/icesugar/synth.sh`) — FIT + TIMING OK
```
Max frequency for clock 'clk_eth': 41.19 MHz (PASS at 12.00 MHz)
Max frequency for clock 'clk_sys': 13.48 MHz (PASS at 12.00 MHz)
icesugar: fit + timing OK (ICESTORM_LC 5190/5280; all constrained clocks meet timing)
```

### Shared-code elaboration (default rx_enable=true)
The default-true path is exercised by every non-iCESugar board's soc_gen +
by `uartlitedb`'s default generic map. The iCESugar sim itself analyzes and
elaborates `uart.vhd`/`uartlitedb.vhd` under GHDL `--std=93` (with rx_enable
bound FALSE for uart0); the true-branch guards are the original code verbatim.

## Files changed
- `components/uartlite/uart.vhm` (canonical) + `components/uartlite/uart.vhd` (generated)
- `components/uartlite/uart_pkg.vhd`
- `components/uartlite/uartlitedb.vhd`
- `targets/boards/icesugar/design.yaml` (`rx_enable: false` on uart0)
- regenerated `targets/boards/icesugar/devices.vhd` (+ other soc_gen outputs)
