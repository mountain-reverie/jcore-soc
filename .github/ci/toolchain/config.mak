# musl-cross-make config for the J-core Linux cross-toolchain.
# Target: SH-2 big-endian, musl libc, FDPIC (nommu J-core ABI).
# Binutils is overridden to our jcore fork via a pre-staged tarball (see Dockerfile Task 2).
TARGET = sh2eb-linux-muslfdpic

# Pinned stock upstream versions (musl-cross-make known-good defaults).
BINUTILS_VER = jcore
GCC_VER = 9.4.0
MUSL_VER = 1.2.6
GMP_VER = 6.3.0
MPC_VER = 1.3.1
MPFR_VER = 4.2.2
LINUX_VER = headers-4.19.88-2

# C only (no C++ — YAGNI); keep libgcc + libatomic (SH needs -latomic_asneeded).
GCC_CONFIG += --enable-languages=c
GCC_CONFIG += --disable-libquadmath --disable-libstdcxx --disable-nls
COMMON_CONFIG += --disable-nls
