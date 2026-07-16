# J-core Linux cross-toolchain image

Builds and publishes **`ghcr.io/mountain-reverie/jcore-linux-toolchain`** — an
`sh2eb-linux-muslfdpic` cross-toolchain (SH-2 big-endian, musl libc, FDPIC) that can
compile **and link** a full j-core Linux `vmlinux`. Other GitHub Actions consume it
(SP3-a boot-prep, and eligible SP1/SP2 jobs) instead of rebuilding a toolchain per run.

## What's in it

Assembled by [musl-cross-make](https://github.com/richfelker/musl-cross-make):

| Component | Source | Notes |
|-----------|--------|-------|
| binutils  | **`mountain-reverie/binutils-gdb` @ `jcore`** | J-core ISA (`cas.l`, J2/J4 split, MMU opcodes), `elf32-shbig-linux`, and the arch-merge fix (`sh-j4` × `sh2a-nofpu-or-sh3-nommu`) a full vmlinux link needs. Pre-staged as a named source tarball (recipe B — musl-cross-make has no override hook; see `OVERRIDE-RECIPE.md`). |
| gcc       | **13.3.0** (pinned stock) | 9.4.0 (mcm default) predates the SH-FDPIC backend fixes and mis-codegens SH; `--enable-languages=c`, no `-mj2` (single-core j2 doesn't need `cas.l` codegen; SMP/`-mj2` is a later gcc task). |
| musl      | pinned (mcm default) | FDPIC, matches nommu j2. |

Multi-stage build: the toolchain compiles into `/opt/sh2eb`; the slim final stage copies
only that plus kernel-build tooling (~1 GB, vs ~4 GB single-stage). Cold build ~45-90 min.

## Files

- `config.mak` — musl-cross-make config (target + version pins + `BINUTILS_VER = jcore`).
- `Dockerfile` — stages the fork binutils tarball, runs musl-cross-make, slims the image.
- `smoke.sh` — `-m2` big-endian compile/link + a `cas.l` assemble (proves the fork binutils).
- `vmlinux-smoke.sh` — builds a full j2 `vmlinux` and validates the ELF (the real proof).
- `OVERRIDE-RECIPE.md` — how the fork binutils is injected into musl-cross-make.

## Using it in another workflow

```yaml
jobs:
  build:
    runs-on: ubuntu-24.04
    container: ghcr.io/mountain-reverie/jcore-linux-toolchain:latest
    steps:
      - run: sh2eb-linux-muslfdpic-gcc --version
```

or `docker pull ghcr.io/mountain-reverie/jcore-linux-toolchain:latest` and `docker run`.

## Building a vmlinux with it

```sh
make ARCH=sh CROSS_COMPILE=sh2eb-linux-muslfdpic- j2_defconfig
make ARCH=sh CROSS_COMPILE=sh2eb-linux-muslfdpic- -j"$(nproc)" vmlinux
```

Use the **`jcore`** branch of `mountain-reverie/linux`: it carries the `CONFIG_CPU_SH2`
`nstree.o -fno-inline` workaround for an upstream gcc SH-backend ICE (`change_address_1`)
that otherwise makes any current-mainline SH kernel unbuildable. Remove that workaround
once the gcc bug is fixed.

## Rebuilding / bumping a version

Edit `config.mak` (or the Dockerfile ARGs) and push to `master` — the
`build-toolchain-image.yml` workflow rebuilds and republishes (`:latest` + `:sha-<sha>`).
It also runs `smoke.sh` and the full `vmlinux` proof as gates. It can also be run manually
via `workflow_dispatch`.
