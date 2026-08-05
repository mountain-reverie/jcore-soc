all: help

# components from component/ whose VHDL is included in builds
COMPONENTS :=
COMPONENTS += clk
COMPONENTS += cpu
COMPONENTS += ddr
COMPONENTS += ddr2
COMPONENTS += dma
COMPONENTS += cpu/cache
COMPONENTS += misc
COMPONENTS += uartlite
COMPONENTS += emac
COMPONENTS += ring_bus
COMPONENTS += gps_if2

# libraries from lib/ whose VHDL is included in builds
LIBS :=
LIBS += hwutils
LIBS += reg_file_struct
LIBS += memory_tech_lib
LIBS += fixed_dsm_pkg

VHDL_DIRS := targets
VHDL_DIRS += $(addprefix components/,$(COMPONENTS))
VHDL_DIRS += $(addprefix lib/,$(LIBS))

# directories to run make in to build tests
TEST_DIRS := components/cpu/tests
TEST_DIRS += components/misc/tests

# some tests need to be built differently
TEST_DIRS2 :=

# directories to run make clean in
CLEAN_DIRS := $(addprefix components/,$(COMPONENTS))
CLEAN_DIRS += $(addprefix lib/,$(LIB))
# only clean those directories that have a Makefile
CLEAN_DIRS := $(foreach d,$(CLEAN_DIRS),$(wildcard $(d)/Makefile))
CLEAN_DIRS := $(dir $(CLEAN_DIRS))
CLEAN_DIRS += tools/tests
CLEAN_DIRS += boot
# ensure all TEST_DIRS are also cleaned
CLEAN_DIRS += $(TEST_DIRS) $(TEST_DIRS2)
CLEAN_DIRS := $(sort $(CLEAN_DIRS))

REVISION := $(shell hg log -r . --template "{latesttag}-{latesttagdistance}-{node|short}" 2>/dev/null || git describe --always --dirty 2>/dev/null || echo unknown)
export REVISION

ISE_VERSION := $(shell xst -help 2>/dev/null | head -1 | sed -n 's/^.*Release \([^ ]*\) .*/\1/p')
export ISE_VERSION

################################################################################
# Gather list of VHDL files
################################################################################

include tools/mk_utils.mk

#$(info VHDL_DIRS $(VHDL_DIRS))

# VHDS_ASIC/VHDS_FPGA scan every VHDL_DIRS entry's build.mk (via mk_utils.mk),
# including components/cpu/build.mk -- which pulls in components/cpu/
# Makefile.inc, which $(eval)s the CPU_DECODE_GENERATED grouped-target RULE
# (a real rule, with a recipe: "$(CPU_DECODE_GENERATED) &: ..."). GNU Make
# does not allow safely $(eval)-ing new RULE definitions from inside a
# recipe's own expansion (a documented gotcha: the makefile database changes,
# but mid-recipe there is no well-defined point to fold a brand-new rule back
# into the dependency graph, and it can outright mis-parse as "prerequisites
# defined in a recipe"). This computation must therefore happen at ordinary
# parse time (an immediate `:=`, evaluated once while make reads this file --
# same as before this refactor), never lazily inside a target's recipe.
#
# CPU_VARIANT-per-board (the whole point of Task 12) is reconciled with that
# constraint below: print-fpga-vhdl-list/print-asic-vhdl-list re-run THIS
# computation, unchanged, in a brand-new `make CPU_VARIANT=<variant> ...`
# process per board (see the $(BOARD_NAMES) recipe) -- a fresh top-level
# parse is exactly the safe context $(eval) needs, and it naturally reuses
# this same code with no duplication.
VHDS_ASIC := $(foreach d,$(VHDL_DIRS),$(call include_asic_vhdl,$(d)))
VHDS_ASIC := $(sort $(VHDS_ASIC))

VHDS_FPGA := $(foreach d,$(VHDL_DIRS),$(call include_fpga_vhdl,$(d)))
VHDS_FPGA := $(sort $(VHDS_FPGA))

print-fpga-vhdl-list:
	@echo $(VHDS_FPGA)

print-asic-vhdl-list:
	@echo $(VHDS_ASIC)

# Per-variant decode generation: components/cpu's Makefile.inc regenerates the
# six cpugen decode sources (decode_pkg/decode/decode_body/decode_table_
# {simple,direct,rom}.vhd) out-of-tree, under DECODE_GEN_DIR (default
# $(CPU_INC_DIR)gen/$(CPU_VARIANT)-w$(CPU_ROM_WIDTH) -- see
# components/cpu/Makefile.inc), so a J4 board gets its own sh4-overlay
# decoder (LDTLB reachable) instead of the committed BASE (J2) decoder.
# Triggered explicitly below (see the $(BOARD_NAMES) recipe) -- these files
# are NOT built as a side effect of merely listing them in VHDL_FILES, since
# the per-board dispatch below spawns `make -C output/<board>` as a
# brand-new make process that never scans components/cpu at all.
#
# The six generated filenames themselves are NOT duplicated here (they used
# to be, plus copy-pasted again into three board shell scripts -- the same
# duplication disease this refactor exists to treat): Makefile.inc's own
# `cpu-decode-gen` phony target is the one name every caller needs, so
# CPU_ROM_WIDTH's default is the only thing that must still agree with
# Makefile.inc's default (72) for `make <board>` without an explicit
# override to behave the same as a direct `make -f components/cpu/build.mk
# cpu-decode-gen` call.
CPU_ROM_WIDTH ?= 72

################################################################################
# Running Tests
################################################################################

# Gather the contents of all TESTS files into a single TESTS file so
# runtests can run them all at once. Alternatively, could modify
# runtests to accept multiple file names.
TEST_BINS := $(foreach d,$(TEST_DIRS) $(TEST_DIRS2),$(addprefix $(d)/,$(shell cat $(firstword $(wildcard $(d)/test_bins) $(wildcard $(d)/TESTS) /dev/null))))
test_bins: force
	rm -f $@
	for t in $(TEST_BINS); do echo "$$t" >> $@; done

build_tests:
	for d in $(TEST_DIRS); do make -C "$$d" || exit 1; done
	for d in $(TEST_DIRS2); do make -C "$$d" taptests || exit 1; done

check: test_bins tools/tests/runtests build_tests
	tools/tests/runtests test_bins

tap: test_bins tools/tests/runtests build_tests
	tools/tests/runtests -t test_bins
# gather the tap files
	rm -rf tap
	mkdir tap
	for t in $(TEST_BINS); do mkdir -p `dirname "tap/$$t"` && cp "$$t.tap" `dirname "tap/$$t"`; done

################################################################################
# Builds boards
################################################################################

# Boards are subdirectories of targets/boards. The tools, environment
# and steps required to build a board are controlled by the Makefile
# in each board direcotry. This soc_top Makefile does four things to
# support the individual boards:
#
# 1. Finds the list of boards and creates Makefile targets for them in
# this Makefile.
#
# 2. When building a board, creates an output directory with a unique
# name which will be the worked directory of the build.
#
# 3. Exports several environment variables that the board makefile can
# use, including the list of all VHDL files.
#
# 4. Dispatch to the board Makefile
#

BOARD_NAMES := $(dir $(wildcard targets/boards/*/Makefile))
BOARD_NAMES := $(foreach D,$(BOARD_NAMES),$(D:%/=%))
BOARD_NAMES := $(notdir $(BOARD_NAMES))
BOARD_NAMES := $(sort $(BOARD_NAMES))
#$(info BOARD_NAMES: $(BOARD_NAMES))

$(BOARD_NAMES): REL_OUTPUT_DIR=output/$@
$(BOARD_NAMES): BOARD_NAME = $@
$(BOARD_NAMES): BOARD_DIR = $(abspath targets/boards/$@)
$(BOARD_NAMES): TOP_DIR := $(abspath .)
$(BOARD_NAMES): TOOLS_DIR := $(abspath tools)

# CPU_VARIANT comes from the board's soc_gen-generated build.mk (from
# design.yaml's `model:`).
#
# PATH: the generated build.mk is targets/boards/<board>/build.mk -- NOT
# targets/boards/<board>/generated/build.mk. (generated/ holds cpus_config.vhd,
# cpu_synth_files.list and friends, but not build.mk.) Reading the wrong path
# yields an empty result and silently defaults every board to j2.
#
# ABSENCE IS NORMAL, NOT AN ERROR: only boards with a `cpu:` block in design.yaml
# get a CPU_VARIANT line. Today ulx3s (j2), icesugar (j1) and gf180_j4mmu (j4)
# have one; microboard, mimas_v2 and turtle_1v0 legitimately do not. So a missing
# CPU_VARIANT must NOT warn on every build -- half the boards would emit it every
# time and the warning would be trained into background noise. Default silently
# to j2 (the historical behaviour, which is what the union list gave them) and
# reserve loud output for a board that HAS a cpu block whose variant is unknown
# (handled by components/cpu/Makefile.inc's own $(error) on an unrecognized
# CPU_VARIANT, triggered when the per-board VHDL_FILES/decode-gen below runs).
define board_cpu_variant
$(strip $(or \
  $(shell sed -n 's/^CPU_VARIANT *:= *//p' targets/boards/$(1)/build.mk 2>/dev/null), \
  j2))
endef

# CPU_VARIANT itself (just a string) IS safe as an ordinary target-specific
# variable -- it never triggers an eval of new rules by itself.
$(BOARD_NAMES): CPU_VARIANT = $(call board_cpu_variant,$@)

$(BOARD_NAMES): tools
# Per-board VHDL_FILES/VHDL_FILES_ASIC: re-invoke this same make with
# CPU_VARIANT overridden on the command line (see the VHDS_ASIC/VHDS_FPGA
# comment above for why this can't just be a lazily-expanded recipe variable).
# Also regenerate this board's decode sources here, in a separate recursive
# invocation of components/cpu/build.mk directly (VHDLS=... satisfies the
# mk_utils.mk indirect-append idiom build_core.mk requires): the final
# dispatch below spawns `make -C output/$@ ...`, a brand-new make process
# that never scans components/cpu (it only sees the already-expanded
# VHDL_FILES TEXT written into output/$@/Makefile), so it cannot build files
# whose rule lives solely in components/cpu/Makefile.inc. The grouped-target
# rule Makefile.inc defines is keyed on variants.toml/spec/cpugen-source
# timestamps, not on CPU_VARIANT's or CPU_ROM_WIDTH's value, so
# DECODE_GEN_DIR's default per-(variant, width) subdirectory
# (gen/$(CPU_VARIANT)-w$(CPU_ROM_WIDTH)) is what keeps two boards on
# different variants (or the same variant at different ROM widths) from
# stepping on (or silently reusing) each other's decoder. `cpu-decode-gen`
# is components/cpu/Makefile.inc's own phony target naming the six files
# generated for whatever CPU_VARIANT/CPU_ROM_WIDTH are passed on this
# command line -- the one name this Makefile needs to know, instead of
# duplicating CPU_DECODE_GEN_NAMES' six literal filenames here too.
	$(MAKE) -f components/cpu/build.mk VHDLS=CPU_DECODE_BUILD_TMP CPU_VARIANT=$(CPU_VARIANT) CPU_ROM_WIDTH=$(CPU_ROM_WIDTH) cpu-decode-gen
# MAKEFLAGS= / MFLAGS= : $(shell $(MAKE) ...) is a plain string substitution,
# not GNU Make's special recursive-make recipe handling, so it does not get a
# fresh jobserver -- it just inherits whatever MAKEFLAGS is already in this
# process's environment (e.g. a jobserver --jobserver-auth=R,W fd pair from an
# ANCESTOR make). That's fine for an ordinary recursive `$(MAKE) target`
# recipe line, but here we're capturing this sub-make's STDOUT as a string;
# when this whole chain is itself invoked from inside another make's recipe
# (e.g. soc_gen's Go tool shelling back out to `make ... TARGET=vhdl_list.txt`,
# which is exactly board.Files()'s path in tools/socgen/board/board.go),
# the inherited jobserver fds are not valid in that new process tree and GNU
# Make chokes ("multiple target patterns" is one observed symptom). Clearing
# MAKEFLAGS/MFLAGS for just this sub-invocation avoids inheriting a jobserver
# handle that doesn't apply here.
	$(eval VHDL_FILES := $(shell MAKEFLAGS= MFLAGS= $(MAKE) -s CPU_VARIANT=$(CPU_VARIANT) print-fpga-vhdl-list))
	$(eval VHDL_FILES_ASIC := $(shell MAKEFLAGS= MFLAGS= $(MAKE) -s CPU_VARIANT=$(CPU_VARIANT) print-asic-vhdl-list))
	mkdir -p "$(REL_OUTPUT_DIR)"
	echo "REVISION:=$(REVISION)" >> "$(REL_OUTPUT_DIR)/Makefile.tmp"
	echo "ISE_VERSION:=$(ISE_VERSION)" >> "$(REL_OUTPUT_DIR)/Makefile.tmp"
	echo "BOARD_NAME:=$(BOARD_NAME)" >> "$(REL_OUTPUT_DIR)/Makefile.tmp"
	echo "BOARD_DIR:=$(BOARD_DIR)" >> "$(REL_OUTPUT_DIR)/Makefile.tmp"
	echo "TOP_DIR:=$(TOP_DIR)" >> "$(REL_OUTPUT_DIR)/Makefile.tmp"
	echo "OUTPUT_DIR:=$(TOP_DIR)/$(REL_OUTPUT_DIR)" >> "$(REL_OUTPUT_DIR)/Makefile.tmp"
	echo "TOOLS_DIR:=$(TOOLS_DIR)" >> "$(REL_OUTPUT_DIR)/Makefile.tmp"
	echo "VHDL_FILES:=$(VHDL_FILES)" >> "$(REL_OUTPUT_DIR)/Makefile.tmp"
	echo "VHDL_FILES_ASIC:=$(VHDL_FILES_ASIC)" >> "$(REL_OUTPUT_DIR)/Makefile.tmp"
	echo "include ../../targets/boards/$@/Makefile" >> "$(REL_OUTPUT_DIR)/Makefile.tmp"
	printf 'print-vhdl-files:\n\t@echo $$(VHDL_FILES)\n' >> "$(REL_OUTPUT_DIR)/Makefile.tmp"
	test -e "$(REL_OUTPUT_DIR)/Makefile" && cmp "$(REL_OUTPUT_DIR)/Makefile.tmp" "$(REL_OUTPUT_DIR)/Makefile" || mv "$(REL_OUTPUT_DIR)/Makefile.tmp" "$(REL_OUTPUT_DIR)/Makefile"
	rm -f "$(REL_OUTPUT_DIR)/Makefile.tmp"
	make -C "$(REL_OUTPUT_DIR)" $(TARGET)

################################################################################
# tools
################################################################################

tools: tools/tests/runtests
	make -C tools/genram

tools/tests/runtests: force
	make -C tools/tests

# soc_gen generates each board's SoC files (devices.vhd, soc.vhd, pad_ring.vhd,
# board.dts, board.h, build.mk, and word_ack_gen.vhd where applicable) with the
# Go tool under tools/socgen. It replaced the retired Clojure phase-1 + csh
# phase-2. Boards default to every targets/boards/*/ that has a design.yaml;
# override with `make soc_gen BOARDS="turtle_1v0 ..."`.
SOCGEN_BOARDS := $(notdir $(patsubst %/,%,$(dir $(wildcard targets/boards/*/design.yaml))))

soc_gen:
	@command -v go >/dev/null 2>&1 || (printf "***************************************************************************\n****** Go (https://go.dev/dl/) is required to run the soc_gen tool.   ******\n***************************************************************************\n" && false)
	@for b in $(if $(BOARDS),$(BOARDS),$(SOCGEN_BOARDS)); do \
		echo "soc_gen $$b"; \
		(cd tools/socgen && go run ./cmd/socgen -root "$(CURDIR)" "$$b") || exit 1; \
	done
	@echo "Done"

help:
	@echo "To build a bitstream for a specific board, run 'make <BOARDNAME>' where <BOARDNAME> is one of"
	@for b in $(BOARD_NAMES); do echo "  - $$b"; done
	@echo ""

	@echo "'make check' will build and run a series of GHDL simulation tests"

	@echo ""
	@echo "A tool named soc_gen generates some of the VHDL that goes into the bitstreams"
	@echo "'make soc_gen' will run soc_gen for all boards"
	@echo "'make <BOARDNAME> TARGET=soc_gen' runs soc_gen for a specific board"

	@echo ""
	@echo "See the README file for more information."

clean:
	rm -f test_bins
	rm -rf tap
	for d in $(CLEAN_DIRS); do make -C "$$d" clean || exit 1; done

.PHONY: all help clean force check tap build_tests tools soc_gen $(BOARD_NAMES)
