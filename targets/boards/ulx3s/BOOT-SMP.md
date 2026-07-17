# Booting 2-core SMP j2 Linux on the J-Core ULX3S (`j2-dual`)

This runbook builds a bootable microSD package for the dual-core **`j2-dual`** ULX3S SoC
and boots SMP j2 nommu Linux — both cores online, cross-core IPI. It is the SMP
counterpart to `BOOT.md` (single-core `j2-direct`). Everything up to "Boot on hardware"
is mechanically verifiable; the actual 2-core boot is your bench step.

## How SMP bring-up works (what's different from single-core)

Linux J-core SMP (`arch/sh/kernel/cpu/sh2/smp-j2.c`) is complete. Two mechanisms:
- **Secondary release (spin-table):** cpu1 is held halted (the boot ROM leaves it so —
  `CONFIG_CPU1_DIAG=0`). The kernel writes cpu1's entry PC to the mailbox `0x8000` and
  sets the enable at `0xabcd0640` (`cpu-release-addr = <0xabcd0640 0x8000>` in the DT);
  cpu1 leaves halt and runs the secondary path.
- **IPI (inter-processor interrupt):** `j2_send_ipi(cpu)` writes bit-28 of a per-cpu
  trigger word (`writel(readl(ipi+cpu)|(1<<28), ipi+cpu)`); that pulses the target core's
  interrupt. The IPI block is the **`icache_modereg`** entity (the same one the turtle
  board uses for SMP), made a socgen-generic `ipi` device this cycle and wired into
  `j2-dual` (`ipi { compatible="jcore,ipi-controller"; reg=<0xabcd00c0 0x8>;
  interrupts=<0x14>; }` in the generated DT).

Both are **RTL-cosim-verified** per-PR in jcore-cpu (`smp_bringup` on `cpu_dualcore_tb`:
spin-table release + IPI bit-28, auto-clearing). The boot ROM needs no SMP handshake — it
only leaves cpu1 halted; the kernel's spin-table owns the release.

## Prerequisites (host)

Same as `BOOT.md`: the `ghcr.io/mountain-reverie/jcore-linux-toolchain:latest` image,
`dtc`, `mtools`, `sfdisk`, `sh2-elf-gcc` (boot ROM), and a ULX3S flashing tool.

## 1. Build the SMP `vmlinux` (linux `jcore`, `j2_defconfig`)

```sh
git clone -b jcore https://github.com/mountain-reverie/linux.git
docker run --rm -v "$PWD/linux:/linux:ro" -v "$PWD/kb:/kb" \
  ghcr.io/mountain-reverie/jcore-linux-toolchain:latest sh -c '
    cd /linux
    make O=/kb ARCH=sh CROSS_COMPILE=sh2eb-linux-muslfdpic- j2_defconfig
    make O=/kb ARCH=sh CROSS_COMPILE=sh2eb-linux-muslfdpic- -j"$(nproc)" vmlinux'
# -> kb/vmlinux : CONFIG_SMP=y, CONFIG_NR_CPUS=2, symbols j2_smp_ops + start_secondary
```
(`j2_defconfig` already has `CONFIG_SMP=y`; `CONFIG_NR_CPUS=2` is pinned. If the linux
source tree was ever built in-tree, run `make ARCH=sh mrproper` first — the O= build
refuses a dirty source tree.)

## 2. Generate the SMP device tree

```sh
make ulx3s TARGET=soc_gen VARIANT=j2-dual        # board.dts gets cpu@1 + enable-method + ipi
dtc -I dts -O dtb -o /tmp/dt-dual.dtb targets/boards/ulx3s/board.dts
make ulx3s TARGET=soc_gen VARIANT=j2-direct      # restore the committed (default) board.dts
```
The `j2-dual` DT carries `cpu@1`, `enable-method="jcore,spin-table"`,
`cpu-release-addr=<0xabcd0640 0x8000>`, the `cpuid` node, and the `jcore,ipi-controller`
`ipi` node.

## 3. Build the boot ROM (cpu1 halted for the kernel)

```sh
make -C targets/boards/ulx3s/rom clean all       # CONFIG_CPU1_DIAG=0: cpu1 stays halted
```

## 4. Get the `j2-dual` bitstream

**Use the SP3-d branch's bitstream** — it contains the new IPI block (`icache_modereg` +
`aic_irq_combine`). The pre-SP3-d master bitstream has no IPI hardware and will boot
single-core only.

```sh
# from the SP3-d jcore-soc PR's board-synth run:
gh run download --repo mountain-reverie/jcore-soc -n ulx3s-j2-dual-bitstream   # -> ulx3s.bit (~2 MB)
```
(or `VARIANT=j2-dual ./targets/boards/ulx3s/synth.sh` on the SP3-d branch).

## 5. Build the SD image

```sh
targets/boards/ulx3s/mk_sd_image.sh -o sd-dual.img -s 64 /path/to/vmlinux /tmp/dt-dual.dtb
```

## 6. Boot on hardware (your bench)

```sh
sudo dd if=sd-dual.img of=/dev/sdX bs=4M conv=fsync     # X = your microSD device!
openFPGALoader -b ulx3s ulx3s.bit                        # the SP3-d j2-dual bitstream
```
Serial at **115200 8N1**: boot ROM banner → `Loaded vmlinux` → kernel banner → SMP bring-up
(`Booting CPU1` / `Brought up 2 CPUs`). With no rootfs (deferred), the target is both cores
online reaching `init`; a userspace shell needs the FDPIC-musl userspace increment.

## Verification status (mechanical, done here)

| # | Check | Result |
|---|-------|--------|
| 1 | SMP `vmlinux` builds; `CONFIG_SMP=y`, `NR_CPUS=2`; `j2_smp_ops`/`start_secondary` present | PASS |
| 2 | Secondary release + IPI (bit-28→irq, auto-clear) on 2-core RTL | PASS (jcore-cpu `smp_bringup` cosim, non-vacuous) |
| 3 | `j2-dual` DT: `cpu@1` + `enable-method` + `cpu-release-addr` + `jcore,ipi-controller` (reg 0xabcd00c0, irq 0x14) | PASS (dtc clean) |
| 4 | `j2-dual` bitstream synthesizes with the IPI block (timing-gated) | via the SP3-d PR board-synth |
| 5 | `sd-dual.img` well-formed FAT16 with SMP `vmlinux` + `dt-dual.dtb` | PASS (mdir/sfdisk) |
| 6 | rom builds `CONFIG_CPU1_DIAG=0` (cpu1 halted) | PASS |

**Not verified here:** the actual 2-core boot (both cores online, cross-core scheduling) —
the bench step. A full SMP boot cannot be RTL-sim'd; SP3-d hands off a verified-consistent
package plus the `smp_bringup` cosim evidence that release + IPI are sound.
