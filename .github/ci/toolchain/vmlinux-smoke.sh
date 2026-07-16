#!/bin/sh
# Build a full j2 nommu vmlinux with the image toolchain and validate the ELF.
# This is the end-to-end proof that the sh2eb-linux-muslfdpic toolchain
# (fork binutils + gcc + musl) can build + LINK a real j-core kernel.
#
# Requires the linux source tree at $LINUX (default /linux). Use the mountain-reverie
# linux `jcore` branch: it carries the CONFIG_CPU_SH2 gcc-SH-ICE workaround for
# kernel/nstree.c (-fno-inline) without which no current-mainline SH kernel builds.
# The fork binutils in this image carries the arch-merge fix (sh-j4 x
# sh2a-nofpu-or-sh3-nommu) that a full vmlinux link needs.
set -eu
LINUX=${LINUX:-/linux}
CROSS=sh2eb-linux-muslfdpic-
OUT=${OUT:-/tmp/kbuild}
mkdir -p "$OUT"
cd "$LINUX"
make O="$OUT" ARCH=sh CROSS_COMPILE=${CROSS} j2_defconfig
make O="$OUT" ARCH=sh CROSS_COMPILE=${CROSS} -j"$(nproc)" vmlinux
test -f "$OUT/vmlinux" || { echo "FAIL: no vmlinux"; exit 1; }

readelf -h "$OUT/vmlinux" | grep -q 'SuperH'          || { echo "FAIL: vmlinux not SuperH"; exit 1; }
readelf -h "$OUT/vmlinux" | grep -qi 'big endian'      || { echo "FAIL: vmlinux not big-endian"; exit 1; }
# PT_LOAD at the J-core memory base 0x10000000.
readelf -l "$OUT/vmlinux" | grep -q '0x10000000'       || { echo "FAIL: no PT_LOAD @0x10000000"; exit 1; }
# Real kernel, not a stub: core symbols must be present.
nm "$OUT/vmlinux" | grep -q ' T start_kernel$'         || { echo "FAIL: no start_kernel"; exit 1; }

echo "VMLINUX OK: entry $(readelf -h "$OUT/vmlinux" | awk '/Entry point/{print $NF}'), arch $(objdump -f "$OUT/vmlinux" | grep -oE 'architecture: [^,]*')"
