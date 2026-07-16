# Booting J4 **MMU** Linux on the J-Core ULX3S (`j4-rom`)

This runbook builds a bootable microSD package for the single-core **`j4-rom`** ULX3S SoC
(a J4 core with the SH-4-class MMU in RTL) and boots the J-Core **MMU** Linux port (the
SP1 TLB-miss handler + `head_32.S` MMU-enable). It is the J4-TLB-in-Linux counterpart to
`BOOT.md` (which boots the j2 nommu kernel). Everything up to "Boot on hardware" is
mechanically verifiable; the boot itself is your bench step.

## How the MMU boot works (differs from j2 nommu)

The kernel is linked at **P1** (`PAGE_OFFSET=0x80000000` + `MEMORY_START=0x10000000` →
entry/link at `0x9000xxxx`), which is **untranslated** (`PA = VA & 0x1FFFFFFF` →
`0x1000xxxx` DRAM). The boot ROM is **MMU-agnostic**: `loadelf` loads each segment to its
`p_paddr` (a P1 address) — writes fold to DRAM in hardware — and jumps to `e_entry` (P1),
which executes from DRAM with AT still off. The kernel then **self-enables the MMU** in
`head_32.S` → `jcore_mmu_enable` (programs TSBBR/TSBCFG/ASIDR/PTEH, then `MMUCR=AT|TI`).
No boot-ROM or bitstream MMU pre-setup is needed — the flash/SD steps are identical to the
nommu flow.

The `head_32.S` MMU-enable sequence is **RTL-cosim-verified** per-PR in jcore-cpu
(`mmuboot` guard: AT enables, a present-page translated access relocates, an absent-page
access takes the TLB-miss vector).

## Prerequisites (host)

Same as `BOOT.md`: the `ghcr.io/mountain-reverie/jcore-linux-toolchain:latest` image,
`dtc`, `mtools`, `sfdisk`, `sh2-elf-gcc` (boot ROM), and a ULX3S flashing tool.

## 1. Build the MMU `vmlinux` (linux `jcore` + `jcore_defconfig`)

```sh
git clone -b jcore https://github.com/mountain-reverie/linux.git
docker run --rm -v "$PWD/linux:/linux" \
  ghcr.io/mountain-reverie/jcore-linux-toolchain:latest sh -c '
    cd /linux
    make ARCH=sh CROSS_COMPILE=sh2eb-linux-muslfdpic- jcore_defconfig
    make ARCH=sh CROSS_COMPILE=sh2eb-linux-muslfdpic- -j"$(nproc)" vmlinux'
# -> linux/vmlinux : SH big-endian, MMU, entry 0x9000xxxx (P1), PT_LOAD vaddr/paddr 0x9000xxxx
```
`jcore_defconfig` selects `CPU_SUBTYPE_JCORE` → `MMU` + 16KB pages + P1 `PAGE_OFFSET`, plus
the console/spi/mmc/fs drivers. The `jcore` branch carries the `nstree.o -fno-inline`
gcc-ICE workaround and the `elf32-shbig-linux` ld_bfd fix needed by this toolchain.

## 2. Compile the j4 device tree

```sh
cd targets/boards/ulx3s
dtc -I dts -O dtb -o dt-j4.dtb board-j4.dts     # cpu@0 = "jcore,j4"; peripherals per SP3-a
```

## 3. Build the boot ROM (unchanged — MMU-agnostic)

```sh
make -C targets/boards/ulx3s/rom clean all      # same boot.bin as the j2 flow
```

## 4. Get the `j4-rom` bitstream

```sh
gh run download --repo mountain-reverie/jcore-soc -n ulx3s-j4-rom-bitstream   # -> ulx3s.bit (~2 MB)
```
(or `VARIANT=j4-rom ./targets/boards/ulx3s/synth.sh`). This variant binds
`MMU_ARCH=true, PRIV_ARCH=true` — the MMU is in the fabric.

## 5. Build the SD image

```sh
targets/boards/ulx3s/mk_sd_image.sh -o sd-j4.img -s 64 /path/to/vmlinux \
  targets/boards/ulx3s/dt-j4.dtb
```

## 6. Boot on hardware (your bench)

```sh
sudo dd if=sd-j4.img of=/dev/sdX bs=4M conv=fsync      # X = your microSD device!
openFPGALoader -b ulx3s ulx3s.bit                       # load the j4-rom bitstream
```
Insert the microSD, connect USB serial at **115200 8N1**, watch for: boot ROM banner →
`Loaded vmlinux` → the kernel `earlycon` banner → (MMU enables in `head_32.S`) →
`start_kernel`. With no rootfs (deferred), the expected first-boot outcome is the kernel
coming up and reaching `init` (an init-not-found panic over earlycon proves the kernel +
MMU are alive); a userspace shell needs the FDPIC-musl userspace increment.

## Verification status (mechanical, done here)

| # | Check | Result |
|---|-------|--------|
| 1 | MMU `vmlinux` builds+links; entry P1 (`0x9000xxxx`), MMU on, handler+boot-TSB symbols | PASS |
| 2 | `head_32.S` MMU-enable path (AT + relocation + miss) | PASS (jcore-cpu `mmuboot` cosim, non-vacuous) |
| 3 | `dt-j4.dtb` compiles; `cpu@0 = jcore,j4`; peripherals kernel-consistent | PASS |
| 4 | `j4-rom` bitstream synthesizes (`MMU_ARCH=true`, timing-gated) | PASS (CI, ~2 MB) |
| 5 | `sd-j4.img` well-formed FAT16 with MMU `vmlinux` + `dt-j4.dtb` | PASS (mdir/sfdisk) |
| 6 | ELF `p_paddr`/`e_entry` (P1) match the MMU-agnostic boot ROM load/jump | PASS |

**Not verified here:** the actual hardware boot (the bench step) — booting a full MMU
kernel in GHDL sim is infeasible. SP3-c hands off a verified-consistent package plus the
`mmuboot` cosim evidence that the MMU-enable path is sound.
