CROSS_COMPILE ?= sh2-elf-
CC      := $(CROSS_COMPILE)gcc
OBJCOPY := $(CROSS_COMPILE)objcopy
CFLAGS  := -m2 -O2 -ffreestanding -fno-builtin -nostdlib -Wall

all: main.bin

main.elf: start.S banner.c ulx3s.ld
	$(CC) $(CFLAGS) -T ulx3s.ld -o $@ start.S banner.c

main.bin: main.elf
	$(OBJCOPY) -O binary $< $@

clean:
	rm -f main.elf main.bin
