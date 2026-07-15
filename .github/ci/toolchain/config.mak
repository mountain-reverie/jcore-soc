# musl-cross-make config for the J-core Linux cross-toolchain.
# Target: SH-2 big-endian, musl libc, FDPIC (nommu J-core ABI).
# Binutils is overridden to our jcore fork via a pre-staged tarball (see Dockerfile Task 2).
TARGET = sh2eb-linux-muslfdpic

# Pinned stock upstream versions (musl-cross-make known-good defaults).
BINUTILS_VER = jcore
# gcc 13.3.0, not musl-cross-make's default 9.4.0: 9.4.0 predates the SH-FDPIC
# backend fixes (mcm ships 0009-sh-fdpic-pr114641.diff etc. only from gcc 10.3.0
# up) and ICEs / emits out-of-range branches building stock mainline SH. 13.3.0
# carries the SH-FDPIC patches and builds a stock-mainline j2 kernel (gcc trunk
# was evaluated for the kernel/nstree.c SH ICE but ICEs identically + its libgcc
# hits a separate hardcfr SH bug, so it is not usable here).
GCC_VER = 13.3.0
MUSL_VER = 1.2.6
GMP_VER = 6.3.0
MPC_VER = 1.3.1
MPFR_VER = 4.2.2
LINUX_VER = headers-4.19.88-2

# C only (no C++ — YAGNI); keep libgcc + libatomic (SH needs -latomic_asneeded).
GCC_CONFIG += --enable-languages=c
GCC_CONFIG += --disable-libquadmath --disable-libstdcxx --disable-nls
COMMON_CONFIG += --disable-nls
