# -DITERATIONS=1000: fixed iteration count. This bare-metal port has no way
# to cheaply auto-calibrate a ~10s run (CoreMark's ITERATIONS==0 auto-detect
# pre-runs the benchmark for real, on real hardware, to time it), so use a
# small fixed count instead, per the barebones-port convention.
GIT_REV := $(shell command git rev-parse --short=8 HEAD 2>/dev/null)
ifeq ($(GIT_REV),)
GIT_REV := 0
else
GIT_REV := 0x$(GIT_REV)
endif

CROSS_COMPILE ?= sh2-elf-
CC := $(CROSS_COMPILE)gcc
OBJCOPY := $(CROSS_COMPILE)objcopy
OBJDUMP := $(CROSS_COMPILE)objdump
CFLAGS := -m2 -mb -O2 -ffreestanding -fno-builtin -nostdlib -Wall \
          -I. -Ivendor -DITERATIONS=1000 -DGIT_REV=$(GIT_REV)
LDSCRIPT := coremark.ld
OBJS := crt0.o core_portme.o eth_report.o \
        vendor/core_main.o vendor/core_list_join.o vendor/core_matrix.o \
        vendor/core_state.o vendor/core_util.o
coremark.elf: $(OBJS) $(LDSCRIPT)
	$(CC) $(CFLAGS) -T $(LDSCRIPT) -o $@ $(OBJS) -lgcc
coremark.bin: coremark.elf
	$(OBJCOPY) -O binary $< $@
%.o: %.S ; $(CC) $(CFLAGS) -c -o $@ $<
%.o: %.c ; $(CC) $(CFLAGS) -c -o $@ $<
clean: ; rm -f *.o vendor/*.o *.elf *.bin
