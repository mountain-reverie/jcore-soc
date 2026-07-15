# SP3-a0 Task 0 — musl-cross-make pins + binutils-source-override recipe

Working note for Task 1 (`config.mak` + Dockerfile). Delete once Task 1 lands.

## musl-cross-make pin

- Upstream: https://github.com/richfelker/musl-cross-make.git
- `MCM_REF` (40-char HEAD sha at clone time): `227df8b99103f9c59f6570babf892978e293082f`

## Stock version pins (from `mcm/Makefile`, NOT `config.mak.dist` — the `.dist`
file only shows a stale illustrative sample (binutils 2.25.1 / gcc 5.2.0);
the real defaults actually used when a var is left unset in `config.mak` are
hardcoded in `Makefile` itself)

```
BINUTILS_VER = 2.44
GCC_VER      = 9.4.0
MUSL_VER     = 1.2.6
GMP_VER      = 6.3.0
MPC_VER      = 1.3.1
MPFR_VER     = 4.2.2
ISL_VER      =            # unset by default; optional, in-tree build skipped if blank
LINUX_VER    = headers-4.19.88-2
```

Patch directories confirming these are real supported pins:
`mcm/patches/binutils-2.44/`, `mcm/patches/gcc-9.4.0/`, `mcm/patches/musl-1.2.6/`
all exist in the clone.

gcc/musl/gmp/mpc/mpfr/isl/linux-headers stay at these stock pins — only
binutils is overridden (see below).

## Target confirmed buildable

`sh2eb-linux-muslfdpic` is a recognized/documented target: it appears
verbatim as a commented example in `mcm/config.mak.dist`:

```
# TARGET = sh2eb-linux-muslfdpic
```

musl-cross-make itself is target-agnostic (the `Makefile`/`litecross/Makefile`
build logic doesn't special-case any target string); SH-fdpic support lives in
the upstream gcc/binutils/musl source trees' own configure logic + the
patches shipped for the pinned versions (e.g. `mcm/patches/gcc-5.3.0/0007-fdpic.diff`,
`0008-shsibcall.diff` for older pins — fdpic/sh-specific patches exist across
multiple pinned gcc/binutils version dirs, confirming fdpic is a
long-supported, actively patched target family in this project, not
something bolted on).

## Binutils-source-override recipe: CHOSEN = (B) named tarball

**`BINUTILS_CUSTOM` does NOT exist in this musl-cross-make version.**
Verified by exhaustive grep across the whole clone:

```
grep -RIn 'CUSTOM' mcm/          # zero hits, anywhere
```

The source-acquisition machinery in `mcm/Makefile` is:

```makefile
$(SOURCES)/%: hashes/%.sha1 | $(SOURCES)
	mkdir -p $@.tmp
	cd $@.tmp && $(DL_CMD) $(notdir $@) $(SITE)/$(notdir $@)
	cd $@.tmp && touch $(notdir $@)
	cd $@.tmp && $(SHA1_CMD) $(CURDIR)/hashes/$(notdir $@).sha1
	mv $@.tmp/$(notdir $@) $@
	rm -rf $@.tmp

%.orig: $(SOURCES)/%.tar.gz    # (also .tar.bz2 / .tar.xz variants)
	...tar xzf...

%: %.orig
	...applies patches/$@/* if the dir exists...
```

i.e. there is no override hook — only a plain "does `sources/binutils-X.tar.gz`
already exist" file-timestamp check before `make` tries to download it.
Pre-staging the tarball + matching sha1 file is therefore the only override
mechanism this musl-cross-make version supports. This is recipe (B) from the
brief, promoted to primary (not merely a fallback) since (A) is unavailable.

### Exact `config.mak` lines (Task 1 to add)

```makefile
TARGET = sh2eb-linux-muslfdpic
BINUTILS_VER = jcore
GCC_VER = 9.4.0
MUSL_VER = 1.2.6
GMP_VER = 6.3.0
MPC_VER = 1.3.1
MPFR_VER = 4.2.2
LINUX_VER = headers-4.19.88-2
```

(`GCC_VER`/`MUSL_VER`/`GMP_VER`/`MPC_VER`/`MPFR_VER`/`LINUX_VER` are only
listed here for clarity/pin-locking in CI; they equal the stock `Makefile`
defaults above and could be omitted, but pinning them explicitly in
`config.mak` guards against a future upstream `Makefile` bump silently
changing versions under us.)

### Exact source-staging commands (Dockerfile / CI step, BEFORE `make`)

```bash
# 1. Fetch our forked binutils source at the pinned ref.
git clone --branch jcore --depth 1 \
    https://github.com/mountain-reverie/binutils-gdb.git binutils-jcore-src

# 2. Package it exactly as musl-cross-make expects: a tarball whose
#    top-level directory is "binutils-<BINUTILS_VER>", i.e. "binutils-jcore".
cd binutils-jcore-src
git archive --format=tar --prefix=binutils-jcore/ HEAD \
    | gzip -9 > ../musl-cross-make/sources/binutils-jcore.tar.gz
cd ..

# 3. Compute the matching sha1 hash file musl-cross-make's SOURCES rule
#    checks against (must exist even though the download rule won't fire,
#    since %.orig / patch application don't depend on the hash file, but
#    a stray `make` re-run that decides to (re)validate/download will look
#    for hashes/binutils-jcore.tar.gz.sha1 and fail hard if it's missing).
sha1sum musl-cross-make/sources/binutils-jcore.tar.gz \
    | awk '{print $1"  binutils-jcore.tar.gz"}' \
    > musl-cross-make/hashes/binutils-jcore.tar.gz.sha1
```

Notes/gotchas for Task 1:

- The tarball's internal top-level directory name **must** be
  `binutils-jcore` (i.e. `binutils-$(BINUTILS_VER)`) — `git archive
  --prefix=binutils-jcore/` enforces this without needing an intermediate
  extract/rename step.
- Because `sources/binutils-jcore.tar.gz` already exists on disk when `make`
  runs, the `$(SOURCES)/%` download rule's target is already satisfied and
  `make` will not attempt to fetch it over the network — no `SITE`/`DL_CMD`
  ever touches it.
- No `patches/binutils-jcore/` directory needs to exist; the fork already
  carries its own changes baked into the tree the tarball is made from. The
  `%: %.orig` rule tolerates a missing `patches/$@` dir (`test ! -d
  patches/$@ || ...`).
- Do the clone/archive/hash steps in a Dockerfile `RUN` stage *before* the
  `make` invocation, in the same layer/step (or an earlier layer copied in)
  so the tarball+hash pair are present before musl-cross-make's `Makefile`
  ever evaluates the `$(SOURCES)/binutils-jcore.tar.gz` rule.
