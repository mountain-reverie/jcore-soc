CROSS_COMPILE ?= sh2-elf-
CC := $(CROSS_COMPILE)gcc
OBJCOPY := $(CROSS_COMPILE)objcopy
CFLAGS := -m2 -mb -O2 -ffreestanding -fno-builtin -nostdlib -Wall -I. -Ivendor
LDSCRIPT := coremark.ld
OBJS := crt0.o main_stub.o
coremark.elf: $(OBJS) $(LDSCRIPT)
	$(CC) $(CFLAGS) -T $(LDSCRIPT) -o $@ $(OBJS)
coremark.bin: coremark.elf
	$(OBJCOPY) -O binary $< $@
%.o: %.S ; $(CC) $(CFLAGS) -c -o $@ $<
%.o: %.c ; $(CC) $(CFLAGS) -c -o $@ $<
clean: ; rm -f *.o *.elf *.bin
