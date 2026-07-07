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

# XIP page-cache software test (Task 6 of the spi_page_cache sub-project):
# xip_handler.s (resident crt0 + _pf_handler) + xip_test.c (>4-page demand-
# paged program) + xip.x (linker script: window VMA 0x10800000 / LMA flash
# 0x100000, SPRAM 0x10000000, resident set in the boot EBR at 0x0). Separate
# from boot.elf/boot.bin above -- this is not the EBR boot program, it is the
# cosim/flash-image target the Task-7 end-to-end sim loads.
XIP_LDSCRIPT := xip.x

xip_test.elf: xip_handler.s xip_test.c $(XIP_LDSCRIPT)
	$(CC) $(CFLAGS) -T $(XIP_LDSCRIPT) -o $@ xip_handler.s xip_test.c

# The flash-resident payload only (xip_test.elf's .text output section,
# which holds xip_test.c's code + the >4-page const table): raw bytes to be
# programmed at flash offset 0x100000 (FLASH_BASE in xip.x), one .img byte
# per flash byte starting at that offset -- i.e. img offset 0 == flash
# offset 0x100000, and img offset (page*4096) == the start of page `page`.
# Does NOT include the resident boot-EBR part (vectors/crt0/_pf_handler),
# which lives in physical EBR/SPRAM address space, not flash.
xip_test.img: xip_test.elf
	$(OBJCOPY) -O binary --only-section=.text $< $@

clean:
	rm -f boot.elf boot.bin xip_test.elf xip_test.img
