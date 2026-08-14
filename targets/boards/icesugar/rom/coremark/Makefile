# -DITERATIONS=100: fixed iteration count. This bare-metal port has no way
# to cheaply auto-calibrate a ~10s run (CoreMark's ITERATIONS==0 auto-detect
# pre-runs the benchmark for real, on real hardware, to time it), so use a
# small fixed count instead, per the barebones-port convention.
#
# 100 rather than 1000: measured on hardware, a 1000-iteration run on the
# 12 MHz J1 takes ~15 minutes, which is a poor CI gate given the board does
# exactly one run per reset. 100 iterations runs in ~90s and still produces
# the CRC, which is what actually validates the gcc build -- the iteration
# count only affects benchmark precision.
#
# It also keeps the run inside the cycle counter's range: CMK CYCLES is a
# 32-bit count at 12 MHz, so it wraps every ~358s. A 1000-iteration run wraps
# it several times and reports a plausible but wrong figure; a 100-iteration
# run does not wrap at all.
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
          -I. -I.. -I../.. -Ivendor -DITERATIONS=100 -DGIT_REV=$(GIT_REV)
LDSCRIPT := coremark.ld
OBJS := crt0.o core_portme.o uart_report.o uart_io.o \
        vendor/core_main.o vendor/core_list_join.o vendor/core_matrix.o \
        vendor/core_state.o vendor/core_util.o
coremark.elf: $(OBJS) $(LDSCRIPT)
	$(CC) $(CFLAGS) -T $(LDSCRIPT) -o $@ $(OBJS) -lgcc
coremark.bin: coremark.elf
	$(OBJCOPY) -O binary $< $@

# cosim.bin: Task 8b GHDL end-to-end cosim payload. Links ONLY crt0 +
# core_portme + uart_report + uart_io + cosim_main (NOT the CoreMark vendor sources),
# reusing the same linker script/startup as the real coremark.bin so it
# boots identically via flash_boot_reader -> SPRAM -> _start -> main.
COSIM_OBJS := crt0.o core_portme.o uart_report.o uart_io.o cosim_main.o
cosim.elf: $(COSIM_OBJS) $(LDSCRIPT)
	$(CC) $(CFLAGS) -T $(LDSCRIPT) -o $@ $(COSIM_OBJS) -lgcc
cosim.bin: cosim.elf
	$(OBJCOPY) -O binary $< $@

uart_io.o: ../uart_io.c ../uart_io.h ; $(CC) $(CFLAGS) -c -o $@ ../uart_io.c
%.o: %.S ; $(CC) $(CFLAGS) -c -o $@ $<
%.o: %.c ; $(CC) $(CFLAGS) -c -o $@ $<
clean: ; rm -f *.o vendor/*.o *.elf *.bin
