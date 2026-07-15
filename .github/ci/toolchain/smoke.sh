#!/bin/sh
# Smoke-test the sh2eb-linux-muslfdpic toolchain inside the image. Exit non-zero on any failure.
set -eu
CROSS=sh2eb-linux-muslfdpic
tmp=$(mktemp -d)
cd "$tmp"

# 1) trivial C compiles + links -m2 big-endian (exercises libgcc + libatomic).
printf 'int main(void){return 0;}\n' > t.c
${CROSS}-gcc -m2 -O2 t.c -o t
${CROSS}-readelf -h t | grep -q 'SuperH' || { echo "FAIL: not SuperH"; exit 1; }
${CROSS}-readelf -h t | grep -qi "big endian" || { echo "FAIL: not big-endian"; exit 1; }
# no-underscore Linux ABI: 'main' symbol has no leading underscore.
${CROSS}-nm t | grep -q ' T main$' || { echo "FAIL: no bare 'main' symbol (ABI?)"; exit 1; }

# 2) A J-core mnemonic assembles -> proves the FORK binutils (stock rejects cas.l).
printf '\t.text\n\tcas.l r1,r2,@r0\n' > j.s
${CROSS}-as j.s -o j.o || { echo "FAIL: cas.l did not assemble (not the jcore binutils?)"; exit 1; }

echo "SMOKE OK"
