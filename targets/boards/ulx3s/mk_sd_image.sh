#!/usr/bin/env bash
# Build a bootable microSD image for the J-Core ULX3S j2-direct boot loader.
#
# The ULX3S boot ROM (CONFIG_SDCARD=1, CONFIG_LOAD_ELF=1, CONFIG_DEVTREE_READ=1)
# mounts FAT partition 1, loads the ELF file "vmlinux", reads "dt.dtb", and jumps
# to the kernel with the DTB pointer in r4. It sets no CONFIG_BOOT_ID, so partition 1
# is used directly (no "id" file needed). Petit-FatFs handles FAT16.
#
# Produces an MBR image with one FAT16 partition holding vmlinux + dt.dtb. Uses
# mtools (mformat/mcopy) so no root / loopback mount is needed.
#   deps: mtools, dosfstools (util-linux sfdisk), coreutils
#
# Usage:
#   ./mk_sd_image.sh [-o sd.img] [-s 64] <vmlinux> <dt.dtb>
# Then flash:  sudo dd if=sd.img of=/dev/sdX bs=4M conv=fsync   (X = your card!)
set -euo pipefail

OUT=sd.img
SIZE_MB=64
while getopts "o:s:h" opt; do
  case "$opt" in
    o) OUT=$OPTARG ;;
    s) SIZE_MB=$OPTARG ;;
    h) sed -n '2,20p' "$0"; exit 0 ;;
    *) echo "bad option" >&2; exit 2 ;;
  esac
done
shift $((OPTIND - 1))

VMLINUX=${1:?usage: mk_sd_image.sh [-o out] [-s MB] <vmlinux> <dt.dtb>}
DTB=${2:?usage: mk_sd_image.sh [-o out] [-s MB] <vmlinux> <dt.dtb>}
[ -f "$VMLINUX" ] || { echo "no vmlinux: $VMLINUX" >&2; exit 1; }
[ -f "$DTB" ]     || { echo "no dtb: $DTB" >&2; exit 1; }
for t in truncate sfdisk mformat mcopy mdir; do
  command -v "$t" >/dev/null || { echo "missing tool: $t (apt install mtools dosfstools util-linux)" >&2; exit 1; }
done

PART_START=2048                        # 1 MiB alignment (sectors, 512 B each)
OFF=$((PART_START * 512))

echo ">> creating ${SIZE_MB} MiB image $OUT"
rm -f "$OUT"
truncate -s "${SIZE_MB}M" "$OUT"

echo ">> writing MBR: 1 FAT16 partition (type 0x0e) from sector ${PART_START}"
sfdisk "$OUT" >/dev/null <<EOF
label: dos
${PART_START},,0e,*
EOF

echo ">> formatting partition as FAT16"
# -F forces FAT32; omit it so mformat picks FAT16 for this size. -v sets a label.
mformat -i "${OUT}@@${OFF}" -v JCORELINUX ::

echo ">> copying vmlinux + dt.dtb"
mcopy -i "${OUT}@@${OFF}" -o "$VMLINUX" ::vmlinux
mcopy -i "${OUT}@@${OFF}" -o "$DTB"     ::dt.dtb

echo ">> contents:"
mdir -i "${OUT}@@${OFF}" ::

echo ">> done: $OUT   (flash with: sudo dd if=$OUT of=/dev/sdX bs=4M conv=fsync)"
