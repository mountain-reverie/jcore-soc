# GF180 2 KB-Cache Journey: Engineering Findings

Branch: `feat/gf180-2kb-cache-padded`. Full measured numbers and methodology
are in [gf180-vs-kianv-comparison.md](gf180-vs-kianv-comparison.md); this doc
captures the *decision arc* — why the SoC ended up on a 2 KB icache+dcache
(the "idcache") path rather than icache-only or a 4 KB cache.

## 1. icache-only was tried first, and abandoned

The most aggressive area cut was to drop the dcache entirely and serve flash
DATA reads uncached, keeping only a (small) icache on the burst-fill XIP
path. This variant **boots and fetches instructions from flash correctly**,
but fails on the very common SH pattern of loading constants out of a
literal pool with `mov.l @(disp,PC), Rn` — an uncached single-word flash
read. Root-caused via VCD waveform inspection: the flash interface delivers
the correct word (`0xf1a5b007` observed on the flash-side bus), but the CPU
register latches an undefined value (`'U'`). The uncached single-word read
was racing the *concurrent* icache line-fill burst for the same flash line,
and the two paths were not arbitrated/ordered against each other.

This is a fixable RTL race (serialize the uncached-read path against an
in-flight line-fill, or fold it into the cache-fill machinery), but SH
compilers route essentially all non-trivial constant loads through the
literal pool, so an uncached data path sits on a hot, whole-program-wide
code path — high risk to fix under a padded-die schedule, and a real
performance tax even once correct. Rather than debug and harden a brand-new
uncached/burst race, the project pivoted to keeping a (small) dcache on the
**already-proven** idcache burst-fill path used successfully by the 8 KB
`one_cpu_idcache` XIP target, and shrinking both caches instead.

## 2. The RAM-primitive floor

`lib/memory_tech_lib`'s existing inferred/vendor RAM primitives bottomed out
at 256-deep tag storage / 2048-deep data storage — i.e. they only covered
`CACHE_INDEX_BITS >= 8` (8 KB and up). Getting below 8 KB required **new**
primitives at every layer the cache touches:

- `RAM_2x8x64` (tag, 64-deep) and `RAM_2x8x512` (data, 512-deep) for the
  first cut, plus
- `RAM_3x8x64` (64-deep but **3×8 = 24 bits wide**, not 2×8 = 16) once the
  tag-width growth below forced a wider tag store.

Each got a TDD equivalence tap (`ram_2x8x64_1rw_tap`, `ram_2x8x512_2rw_tap`,
`ram_3x8x64_1rw_tap` in `lib/memory_tech_lib/tests/`), all wired into
`make check`.

## 3. Tag-width growth: the counterintuitive part

`CACHE_TAG_WIDTH = 28 − 5 − CACHE_INDEX_BITS` (28-bit physical address space,
5 offset bits for a 32-byte line). At 8 KB (`CACHE_INDEX_BITS=8`) the tag is
15 bits — fits the existing 16-bit-wide tag RAM exactly. Shrinking the cache
**widens** the tag: 4 KB and 2 KB overflow the 16-bit tag RAM and need a
24-bit (3×8) tag store instead. This required both a new, wider tag RAM
primitive (`RAM_3x8x64`, above) and parameterizing the cache's tag-storage
width off `CACHE_PA_TAG_WIDTH` rather than hardcoding 2×8.

Net effect: as the data RAM shrinks, the tag RAM *grows* relative to it,
partially offsetting the area win from a smaller cache — worth calling out
explicitly since it is not the direction most people expect.

## 4. Result

With the 2 KB icache + 2 KB dcache on the proven idcache XIP path:

- Boot gates GREEN on both inferred RAM and the GF180 vendor SRAM wrappers
  (`ram_3x8x64_1rw_gf180`, `ram_2x8x512_2rw_gf180`); the 8 KB baseline
  remains byte-identical (regression-safe).
- Measured, routed hardens: `icache_2k` 1.32 mm², `dcache_2k` 1.79 mm²
  placed silicon → composed top **7.89 mm²** (−38 % vs the 8 KB top's
  12.80 mm²) → estimated core die **~12.8 mm²**, landing **~36 % under**
  KianV's 20.1 mm² padded-die reference.

See [gf180-vs-kianv-comparison.md](gf180-vs-kianv-comparison.md) for the full
area table, floorplan history, and basis-of-comparison caveats (placed
silicon vs. padded die vs. core-only).
