# Booting j2 nommu Linux on the J-Core ULX3S (`j2-direct`)

This runbook builds a bootable microSD package for the single-core `j2-direct` ULX3S
SoC and boots upstream j-core Linux (SH-2 nommu) to a serial console. Everything up to
"Boot on hardware" is mechanically verifiable; the boot itself is your bench step.

## How boot works

The ULX3S boot ROM (built from `boot/`, `CONFIG_SDCARD=1 CONFIG_LOAD_ELF=1
CONFIG_DEVTREE_READ=1`) mounts FAT **partition 1**, loads the ELF **`vmlinux`** to its
`p_paddr` (`0x10000000`), reads **`dt.dtb`**, and jumps to the kernel entry with the DTB
pointer in **r4**. No `CONFIG_BOOT_ID` is set, so partition 1 is used directly.

## Prerequisites (host)

- The toolchain image `ghcr.io/mountain-reverie/jcore-linux-toolchain:latest`
  (published by `.github/workflows/build-toolchain-image.yml`).
- `dtc` (device-tree-compiler), `mtools`, `sfdisk`, and `sh2-elf-gcc` (for the boot ROM).
- FPGA flashing tool for the ULX3S (`openFPGALoader` / `fujprog`).

## 1. Build `vmlinux` (linux `jcore` branch, in the toolchain image)

The kernel builds from **`mountain-reverie/linux` `jcore`** — it carries the
`CONFIG_CPU_SH2` `nstree.o -fno-inline` workaround for an upstream gcc SH-backend ICE
(remove once gcc is fixed). The image's fork binutils carries the arch-merge fix a full
vmlinux link needs.

```sh
git clone -b jcore https://github.com/mountain-reverie/linux.git
docker run --rm -v "$PWD/linux:/linux" \
  ghcr.io/mountain-reverie/jcore-linux-toolchain:latest sh -c '
    cd /linux
    make ARCH=sh CROSS_COMPILE=sh2eb-linux-muslfdpic- j2_defconfig
    make ARCH=sh CROSS_COMPILE=sh2eb-linux-muslfdpic- -j"$(nproc)" vmlinux'
# -> linux/vmlinux  (ELF, SH big-endian, entry ~0x10002000, PT_LOAD @0x10000000)
```

## 2. Compile the device tree

```sh
cd targets/boards/ulx3s
dtc -I dts -O dtb -o dt.dtb board.dts     # warnings ok; no errors
```
`board.dts` carries `chosen{stdout-path="serial0:115200n8"; bootargs="console=ttyUL0 earlycon"}`,
`aliases{serial0=&uart}`, and the uartlite `current-speed`/`clock-frequency`.

## 3. Build the boot ROM (delivers the DTB)

```sh
make -C targets/boards/ulx3s/rom clean all   # boot.bin / boot.elf, DEVTREE_READ=1
```

## 4. Get the `j2-direct` bitstream

Either download the CI artifact (recommended)…
```sh
gh run download --repo mountain-reverie/jcore-soc -n ulx3s-j2-direct-bitstream
# -> ulx3s.bit  (~2 MB ECP5 85F bitstream)
```
…or synthesize locally (needs ghdl+yosys+nextpnr-ecp5+ecppack):
```sh
VARIANT=j2-direct ./targets/boards/ulx3s/synth.sh
```

## 5. Build the SD image

```sh
targets/boards/ulx3s/mk_sd_image.sh -o sd.img -s 64 /path/to/vmlinux targets/boards/ulx3s/dt.dtb
# -> sd.img: MBR, FAT16 partition 1 holding vmlinux + dt.dtb
```

## 6. Boot on hardware (your bench)

```sh
sudo dd if=sd.img of=/dev/sdX bs=4M conv=fsync      # X = your microSD device!
openFPGALoader -b ulx3s ulx3s.bit                   # load the j2-direct bitstream
# (the bitstream includes the boot ROM; on power/insert it reads the SD card)
```
Insert the microSD, connect the ULX3S USB serial at **115200 8N1**, and watch for the
boot ROM banner → `Loaded vmlinux` → the kernel `earlycon`/banner → a shell.

## Verification status (mechanical, done here)

| # | Check | Result |
|---|-------|--------|
| 1 | `vmlinux` is a valid SH-BE ELF, `PT_LOAD @0x10000000`, `start_kernel` present | PASS (6.8M, entry 0x10002000, arch `sh-j4`) |
| 2 | `dt.dtb` compiles; `jcore,j2-soc` + peripheral compatibles + `chosen`/console present | PASS |
| 3 | boot ROM builds with `CONFIG_DEVTREE_READ=1 CONFIG_DEVTREE_FILENAME=dt.dtb` | PASS |
| 4 | `j2-direct` bitstream synthesizes (timing-gated) | PASS (CI, ~2 MB `ulx3s.bit`) |
| 5 | `sd.img` is a well-formed FAT16 with `vmlinux` + `dt.dtb` | PASS (mdir/sfdisk) |
| 6 | ELF load/entry (`0x10000000`) + DTB filename match what the ROM expects | PASS |

**Not verified here:** the actual hardware boot (the bench step above). Booting a kernel
in the GHDL RTL sim is infeasible (no SD model; hours–days wall-clock), so this package
is validated for mechanical consistency and hands off to hardware.
