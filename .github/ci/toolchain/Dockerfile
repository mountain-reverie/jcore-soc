# syntax=docker/dockerfile:1
# J-core Linux cross-toolchain: sh2eb-linux-muslfdpic.
# binutils = mountain-reverie/binutils-gdb @ jcore (J-core ISA + elf32-shbig-linux),
# pre-staged as a named source tarball (musl-cross-make has no BINUTILS_CUSTOM
# hook in this pinned version -- see .github/ci/toolchain/OVERRIDE-RECIPE.md).
# gcc/musl = pinned stock upstream via musl-cross-make.
# Cold build ~30-60 min (from-source binutils + 2-stage gcc + musl).
#
# Multi-stage: the builder compiles the toolchain into /opt/sh2eb; the final
# stage copies ONLY /opt/sh2eb into a fresh base, so the multi-GB musl-cross-make
# build tree never lands in a published layer.

ARG UBUNTU_VERSION=24.04

# --------------------------------------------------------------------------
# Stage 1: build the cross-toolchain with musl-cross-make.
# --------------------------------------------------------------------------
FROM ubuntu:${UBUNTU_VERSION} AS builder

ARG MCM_REF=227df8b99103f9c59f6570babf892978e293082f
ARG BINUTILS_REF=jcore
ARG BINUTILS_REPO=https://github.com/mountain-reverie/binutils-gdb.git

ENV DEBIAN_FRONTEND=noninteractive
RUN apt-get update && apt-get install -y --no-install-recommends \
      build-essential git wget curl ca-certificates \
      texinfo file bison flex libgmp-dev libmpc-dev libmpfr-dev \
      python3 rsync \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /build
RUN git clone https://github.com/richfelker/musl-cross-make.git mcm \
    && cd mcm && git checkout ${MCM_REF}

COPY config.mak /build/mcm/config.mak

# Stage our binutils fork as the named BINUTILS_VER=jcore source tarball
# (recipe B: musl-cross-make only supports pre-staging sources/binutils-<VER>.tar.gz,
# there is no override hook -- BINUTILS_CUSTOM does not exist in this pinned version).
# touch the tarball LAST so it is newer than its .sha1 prerequisite; otherwise
# make's timestamp check considers the pre-staged tarball stale and re-downloads
# it (404). See OVERRIDE-RECIPE.md.
RUN mkdir -p /build/mcm/sources /build/mcm/hashes \
    && git clone --branch ${BINUTILS_REF} --depth 1 ${BINUTILS_REPO} /build/binutils-jcore-src \
    && cd /build/binutils-jcore-src \
    && git archive --format=tar --prefix=binutils-jcore/ HEAD \
       | gzip -9 > /build/mcm/sources/binutils-jcore.tar.gz \
    && cd /build/mcm \
    && sha1sum sources/binutils-jcore.tar.gz \
       | awk '{print $1"  binutils-jcore.tar.gz"}' > hashes/binutils-jcore.tar.gz.sha1 \
    && touch sources/binutils-jcore.tar.gz \
    && rm -rf /build/binutils-jcore-src

WORKDIR /build/mcm
RUN make -j"$(nproc)" && make install OUTPUT=/opt/sh2eb

# --------------------------------------------------------------------------
# Stage 2: slim runtime image -- just the toolchain + what a kernel build needs.
# --------------------------------------------------------------------------
FROM ubuntu:${UBUNTU_VERSION}

ENV DEBIAN_FRONTEND=noninteractive
# build-essential = host gcc/make for kbuild HOSTCC + build tools;
# bc/bison/flex/libssl-dev/perl = Linux kernel build prerequisites;
# libgmp10/libmpc3/libmpfr6 = runtime libs the cross gcc's cc1 links against;
# binutils/file = host readelf/nm/file for smoke tests.
RUN apt-get update && apt-get install -y --no-install-recommends \
      build-essential bc bison flex libssl-dev perl \
      libgmp10 libmpc3 libmpfr6 \
      binutils file ca-certificates \
    && rm -rf /var/lib/apt/lists/*

COPY --from=builder /opt/sh2eb /opt/sh2eb
ENV PATH=/opt/sh2eb/bin:${PATH}

CMD ["/bin/bash"]
