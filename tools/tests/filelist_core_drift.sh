#!/usr/bin/env bash
# filelist_core_drift.sh -- every CPU source must reach the ghdl filelists.
#
# WHY: targets/*/filelist.sh are HAND-WRITTEN. They must be, because ghdl needs
# a topological analyse order and components/cpu's build_core.mk fragments are
# unordered make SETS (build_core.mk lists core/cpu.vhd fifth, before
# core/mult_pkg.vhd / core/divider_pkg.vhd / core/datapath_pkg.vhd, and `ghdl -a`
# in that order fails at core/cpu.vhd:5 with "unit decode_pack not found"). So
# the ORDER cannot be generated from them -- but MEMBERSHIP can be checked, and
# that is the failure mode that actually bites:
#
#   core/tlb_walk.vhd was added to build_core.mk when the hardware TSB walker
#   landed. All three filelists missed it, and every board/ASIC synth failed
#   with `unit "tlb_walk" not found in library "work"` -- a submodule bump
#   surfacing as an elaboration error with no obvious connection to the change.
#
# It is queued up to bite again: jcore-cpu's PMU (core/perf_pkg.vhd,
# core/perf.vhd) is in build_core.mk at e8a5a4e, and the pending bump's
# filelists do not mention perf. How that announces itself depends on how the
# CPU refers to the file -- measured with ghdl against e8a5a4e by analyzing the
# ULX3S core block with one file removed:
#
#   * drop core/perf_pkg.vhd -> "unit perf_pack not found in library work" at
#     core/datapath_pkg.vhd:8 (a package behind an unconditional use clause);
#   * drop core/perf.vhd     -> "unit perf not found in library work" at
#     core/cpu.vhd:810 (a DIRECT entity instantiation, still fatal even though
#     it sits inside the PRIV_ARCH generate);
#   * drop core/divider.vhd  -> NOTHING. ghdl -a exits 0. `u_div` is a COMPONENT
#     instantiation, so the component declaration in divider_pkg.vhd satisfies
#     the analyzer and --syn-binding then leaves the divider an unbound black
#     box: silently absent hardware in a design that builds clean and fits.
#
# The first two are loud, but only after a full soc_gen + boot-rom + v2p run has
# paid for the privilege. The third is not loud at all. Hence this check.
#
# HOW IT READS THE TWO SIDES. Both by running the real thing, not by scanning
# text:
#
#   * the CPU side by running GNU make over each build_core.mk. They are make
#     FRAGMENTS using tools/mk_utils.mk's indirect-append idiom ($(VHDLS) names
#     the variable to append to) and build_core.mk already contains a
#     $(patsubst ...) entry, so a regex reader has to re-implement make to stay
#     correct as those files evolve. Undefined variables (CPU_EXTRA_FILES,
#     CPU_CONFIG_FILE, CPU_DECODE_GENERATED, CPU_INC_DIR) expand to empty, which
#     is what the $(VAR) rule below wants. Including build_core.mk directly, not
#     build.mk, is deliberate: build.mk pulls in Makefile.inc, which needs Go,
#     variants.toml and GNU Make 4.3+, and defines rules that regenerate the
#     decoder.
#   * the board side by SOURCING filelist.sh and reading the ${FILES[@]} array
#     it defines, so this compares against the exact argv ghdl is handed. The
#     previous `cat filelist.sh cpu_synth_files.list | grep -qF` scan counted a
#     path mentioned in a COMMENT as present, and could not see that ULX3S
#     analyzes generated/icache_modereg.vhd in place of the components/cpu path.
#
# SCOPE is opt-in per list: a filelist is measured against a given build_core.mk
# only if it already names at least one file from it. iCESugar is EBR-only and
# analyzes no cache VHDL at all, so cache/build_core.mk simply does not apply to
# it -- a design choice, not drift. This mirrors
# components/cpu/scripts/guard_list_drift.sh: do not try to merge the two lists
# (a filelist legitimately carries board VHDL no CPU list knows about, and
# legitimately skips CPU units it never binds), only insist that where a target
# draws from a CPU list at all, it draws the whole list.
#
# A file may be legitimately absent (variant-specific, or an alternate decode
# table this target does not build). Those go in EXCLUDE below WITH A REASON.
# The list rots loudly: adding a CPU source forces you through here.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"; cd "$ROOT"

CPU="components/cpu"
CORE_MK="$CPU/build_core.mk"
CACHE_MK="$CPU/cache/build_core.mk"
for mk in "$CORE_MK" "$CACHE_MK"; do
  [ -f "$mk" ] || { echo "ERROR: $mk missing (submodule not checked out?)" >&2; exit 1; }
done

# Absent by design, for every target -- one shared list, so a reason here has to
# hold for gf180_j4mmu, ulx3s AND icesugar. Keep the reason attached.
EXCLUDE_RE='^(core/cpu_config_common\.vhd|core/register_file_ebr\.vhd|core/shifter_seq\.vhd|decode/decode_table_(direct|rom|simple)_config\.vhd|cache/icache_modereg\.vhd|cache/icache_modereg_wsbu\.vhd|cache/(d|i)cache_cacheable_mux\.vhd)$'
#
#   core/cpu_config_common.vhd : NOT variant-specific (build_core.mk lists it as
#     a plain path for every variant -- the reason that used to stand here,
#     "pulled in per-target via CPU_EXTRA_FILES", was simply wrong). It declares
#     configurations cpu_decode_direct_fpga / cpu_decode_rom_fpga (of `cpu`) and
#     cpu_decode_direct_mmu / _sh2a / _mmu_sh2a (of `decode`). Nothing these
#     three targets analyze binds any of them: in jcore-soc they are named only
#     by targets/cpus_one.vhd and targets/cpus_two_fpga.vhd, the MAKE-driven
#     boards' (mimas_v2 / turtle_1v0 / microboard) cpus configurations, which no
#     filelist.sh here analyzes -- all three bind soc_gen's generated
#     cpus_config.vhd to work.cpu_synth_* instead. It is also not ANALYZABLE by
#     these targets: its first two configurations name BOTH work.cpu_decode_direct
#     and work.cpu_decode_rom, while each target analyzes exactly ONE of
#     decode/decode_table_{direct,rom}_config.vhd. Verified by adding it to the
#     analyzed ULX3S j2-direct library: ghdl -a exits 1 with
#     "core/cpu_config_common.vhd:23:30: unit "cpu_decode_rom" not found in
#     library "work"". It is in build_core.mk for components/cpu's own sim,
#     whose $(CPU_CONFIG_FILE) (core/cpu_config_sim.vhd, core/cpu_config_j2a.vhd)
#     does bind these.
#
#   core/register_file_ebr.vhd : architecture ebr of register_file, the FPGA
#     block-RAM regfile. soc_gen-selected, so it arrives through
#     cpu_synth_files.list when it is wanted at all:
#     tools/socgen/elaborate/cpumap.go names it in the {j1,rom} / {j1,rom,dsp}
#     rows (icesugar) and in cpuSynthFPGAOpt's {j4,rom} row (cpu_synth_j4_rom_ebr,
#     the ULX3S j4-dual FPGA-optimised core0). Everything else -- gf180's
#     cpu_synth_j4, ULX3S's cpu_synth_direct / cpu_synth_j4_rom -- binds
#     register_file(two_bank), and an ASIC target has no block RAM to put it in.
#
#   core/shifter_seq.vhd : architecture seq of shifter, the J1 area-optimised
#     sequential shifter. Every configuration these targets bind picks the other
#     one EXPLICITLY -- cpu_synth_direct, cpu_synth_j4_rom, cpu_synth_j4_rom_ebr
#     and cpu_synth_j4 all contain `for u_shifter : shifter use entity
#     work.shifter(comb)`. Only cpu_synth_j1 / cpu_synth_j1_dsp select seq, and
#     cpumap.go's {j1,rom} / {j1,rom,dsp} rows hand icesugar the file through
#     cpu_synth_files.list.
#
#   decode/decode_table_{direct,rom}_config : each target builds exactly ONE
#     decode table and which one is named by the soc_gen-GENERATED
#     cpu_synth_files.list -- i.e. that selection is already machine-maintained
#     and outside this check's remit. It is also genuinely per-VARIANT, not just
#     per-target: ULX3S j2-direct/j2-dual take direct_config and j4-rom/j4-dual
#     take rom_config, from the same filelist.sh, so no per-target rule could
#     pin it either. Analyzing the unselected one would fail anyway --
#     `configuration cpu_decode_rom` binds decode_table(rom), an architecture
#     that only exists once cpugen's gen/<model>/decode/decode_table_rom.vhd is
#     in the list.
#   decode/decode_table_simple_config : no jcore-soc target can select the
#     SIMPLE decoder at all. cpumap.go's cpuSynth table has rows only for
#     {j2,direct}, {j1,rom}, {j1,rom,dsp}, {j4,direct} and {j4,rom}, and
#     CPUSynthConfig returns "unsupported cpu model/decode/mult combination" for
#     anything else, so no design.yaml can ask for decode: simple.
#
#   cache/icache_modereg.vhd : gf180_j4mmu analyzes it directly, and ULX3S DOES
#     analyze it too -- but as the soc_port_*-attribute-stripped staging copy
#     targets/boards/ulx3s/generated/icache_modereg.vhd that gen_synth_sources.sh
#     writes, not through the components/cpu path this check matches on. The raw
#     file's `attribute soc_port_global_name` applications make ghdl --synth
#     assert (Synth_Attribute_Port, synth-vhdl_decls) in the dual-core netlist.
#     A SUBSTITUTION, not an omission: the ULX3S dual variants really do
#     instantiate it -- design.dual-common.yaml attaches it as socgen's `ipi`
#     device class (targets/boards/common_device_classes.yaml), the cross-core
#     IPI trigger. If the staging copy stops being produced,
#     gen_synth_sources.sh is where that breaks.
#   cache/icache_modereg_wsbu.vhd : architecture with_sbu, the with-SBU sibling.
#     cache_pkg.vhd declares it as a component and nothing in components/cpu
#     instantiates it; in jcore-soc it is instantiated only by
#     targets/byte_bus/icache_modereg_wsbu_slave_process.vhd (whose line in
#     targets/build.mk is commented out) and is reachable from a design only
#     through common_device_classes.yaml's `cache_ctrl_wsbu` class, which no
#     design.yaml in this repo uses.
#
#   cache/{d,i}cache_cacheable_mux.vhd : nothing in jcore-soc instantiates
#     either. Inside components/cpu they are instantiated only by that repo's own
#     testbenches (cache/dcache_color_tb.vhd, cache/icache_cacheable_mux_tb.vhd);
#     core/cpu.vhd mentions them in comments only. These targets' cache paths are
#     targets/ddr_ram_mux/{one,two}_cpu_idcache{,_fpga}.vhd (plus gf180's
#     ddr_ram_mux_one_cpu_idcache_gf180.vhd), which wire the caches to the bus mux
#     directly. Worth spelling out rather than leaving unsaid: cache_pkg.vhd --
#     and gf180's own cache_pkg_2k.vhd spike, which copies the declaration --
#     declares dcache_cacheable_mux as a COMPONENT, so had anything instantiated
#     it, omitting the file would leave an unbound black box under --syn-binding
#     instead of a ghdl error. That is the silent shape described at the top.

# Expand one build_core.mk with GNU make itself. See HOW IT READS THE TWO SIDES.
mk_entries () {  # <path to build_core.mk> -> one raw entry per line
  local mk=$1 out
  out=$(make --no-print-directory -s -f - <<MK
VHDLS := DRIFT_LIST
DRIFT_LIST :=
include $mk
\$(info \$(DRIFT_LIST))
.PHONY: drift-noop
drift-noop: ; @:
MK
  ) || { echo "ERROR: GNU make could not read $mk" >&2; exit 1; }
  printf '%s\n' $out
}

# The list a filelist.sh actually hands ghdl. Sourced in a SUBSHELL: these
# scripts `exit 1` when a prerequisite is missing and set variables of their own,
# and gf180_j4mmu's also prints its FILES array (harmless -- duplicate lines only
# feed a set below).
filelist_files () {  # <path to filelist.sh> -> one path per line, as ghdl sees it
  local fl=$1
  (
    set -euo pipefail
    cd "$ROOT"
    # shellcheck disable=SC1090
    source "$fl"
    printf '%s\n' "${FILES[@]}"
  ) || { echo "ERROR: could not source $fl" >&2; exit 1; }
}

# targets/boards/ulx3s/filelist.sh reads its variant's cpu_synth_files.list from
# the gitignored generated/ dir, which only exists after a build has run
# gen_synth_sources.sh -- and this guard is the FIRST step of the CI job, before
# any soc_gen. Stage just that one file, exactly as gen_synth_sources.sh's own
# `cp` does. Running the whole script instead would also copy v2p outputs that do
# not exist yet on a fresh checkout, silently leaving EMPTY .vhd files behind
# (perl reports "Can't open" and still exits 0). If a build already staged the
# dir, leave it alone: what is staged is precisely what filelist.sh will read.
ULX3S_GEN="targets/boards/ulx3s/generated"
if [ ! -f "$ULX3S_GEN/cpu_synth_files.list" ]; then
  mkdir -p "$ULX3S_GEN"
  cp targets/boards/ulx3s/cpu_synth_files.list "$ULX3S_GEN/cpu_synth_files.list"
fi

rc=0
for fl in targets/asic/gf180_j4mmu/filelist.sh \
          targets/boards/ulx3s/filelist.sh \
          targets/boards/icesugar/filelist.sh; do
  # Everything this filelist hands ghdl that lives inside components/cpu, made
  # cpu-relative. Board VHDL (targets/, lib/, components/uartlite/, ...) is out
  # of scope by construction: no CPU list claims it. Captured into a variable
  # first, not straight into a process substitution, so filelist_files' own
  # `exit 1` cannot be swallowed by a subshell and read as an empty (== drift-
  # free) list.
  fl_raw="$(filelist_files "$fl")"
  declare -A in_list=()
  while read -r f; do
    [ -n "$f" ] && in_list["$f"]=1
  done < <(printf '%s\n' "$fl_raw" | sed -n "s|^$CPU/||p")
  if [ "${#in_list[@]}" -eq 0 ]; then
    echo "ERROR: $fl names no $CPU/ file at all" >&2
    exit 1
  fi

  for mk in "$CORE_MK" "$CACHE_MK"; do
    # cpu-relative prefix: "" for build_core.mk, "cache/" for cache/build_core.mk.
    mk_dir="$(dirname "$mk")"
    if [ "$mk_dir" = "$CPU" ]; then prefix=""; else prefix="${mk_dir#"$CPU"/}/"; fi

    # Plain paths only -- $(VARIABLE) entries (CPU_EXTRA_FILES, CPU_CONFIG_FILE,
    # CPU_DECODE_GENERATED) are variant-selected and are exactly what
    # cpu_synth_files.list supplies, so they are not comparable here.
    files=()
    while read -r e; do
      [ -n "$e" ] || continue
      case "$e" in *'$('*) continue ;; esac
      files+=("$prefix$e")
    done < <(mk_entries "$mk")

    # Opt-in scope: does this filelist draw from this list at all?
    applies=no
    for f in "${files[@]}"; do [ -n "${in_list[$f]+x}" ] && { applies=yes; break; }; done
    [ "$applies" = yes ] || continue

    missing=""
    for f in "${files[@]}"; do
      printf '%s' "$f" | grep -qE "$EXCLUDE_RE" && continue
      [ -n "${in_list[$f]+x}" ] || missing="$missing $f"
    done
    if [ -n "$missing" ]; then
      echo "ERROR: $fl is missing sources from $mk:" >&2
      for m in $missing; do echo "    $CPU/$m" >&2; done
      rc=1
    fi
  done
  unset in_list
done

if [ "$rc" != 0 ]; then
  echo "" >&2
  echo "Add them in dependency order (ghdl analyses in sequence; the make" >&2
  echo "fragments are an unordered set and cannot tell you where), or add an" >&2
  echo "entry to EXCLUDE_RE in this script with the reason it does not apply." >&2
  exit 1
fi
echo "OK: all sources in $CORE_MK and $CACHE_MK appear in every filelist.sh that draws from them"
