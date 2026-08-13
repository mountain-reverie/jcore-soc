# iCESugar EBR boot program: a standalone minimal banner/blink (its own crt0 in
# start.S + linker script icesugar.ld), NOT the boot/ submodule bootloader --
# the up5k EBR (8 KiB) is far too small for the full bootloader, and standalone
# avoids the submodule dependency. sh2-elf-gcc -m2 (the wrapper that maps the
# boot submodule's -mj2 is not needed here).
CROSS_COMPILE ?= sh2-elf-
CC      := $(CROSS_COMPILE)gcc
OBJCOPY := $(CROSS_COMPILE)objcopy

CFLAGS  := -m2 -mb -Os -ffreestanding -fno-builtin -nostdlib -Wall -I. -I..
LDSCRIPT := icesugar.ld

# start.S cannot #include "board.h" itself (the assembler only runs the
# preprocessor, not a C parser, and board.h's struct declarations are plain
# C -- see start.S). Derive the one macro it needs from the generated
# board.h instead, so device presence still has a single source of truth.
ASFLAGS := $(CFLAGS) $(shell grep -q '^#define DEVICE_AIC0_ADDR' ../board.h && echo -DDEVICE_AIC0_ADDR=1)

boot.elf: start.S banner.c uart_io.c uart_io.h $(LDSCRIPT) ../board.h
	$(CC) $(ASFLAGS) -c start.S -o start.o
	$(CC) $(CFLAGS) -T $(LDSCRIPT) -o $@ start.o banner.c uart_io.c -lgcc

boot.bin: boot.elf
	$(OBJCOPY) -O binary $< $@

all: boot.bin

clean:
	rm -f boot.elf boot.bin start.o
