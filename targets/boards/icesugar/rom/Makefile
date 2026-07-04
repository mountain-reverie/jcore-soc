# iCESugar EBR boot program: a standalone minimal banner/blink (its own crt0 in
# start.S + linker script icesugar.ld), NOT the boot/ submodule bootloader --
# the up5k EBR (8 KiB) is far too small for the full bootloader, and standalone
# avoids the submodule dependency. sh2-elf-gcc -m2 (the wrapper that maps the
# boot submodule's -mj2 is not needed here).
CROSS_COMPILE ?= sh2-elf-
CC      := $(CROSS_COMPILE)gcc
OBJCOPY := $(CROSS_COMPILE)objcopy

CFLAGS  := -m2 -mb -Os -ffreestanding -fno-builtin -nostdlib -Wall
LDSCRIPT := icesugar.ld

boot.elf: start.S banner.c $(LDSCRIPT)
	$(CC) $(CFLAGS) -T $(LDSCRIPT) -o $@ start.S banner.c

boot.bin: boot.elf
	$(OBJCOPY) -O binary $< $@

all: boot.bin

clean:
	rm -f boot.elf boot.bin
