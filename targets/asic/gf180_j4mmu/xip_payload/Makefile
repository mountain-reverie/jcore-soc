# targets/asic/gf180_j4mmu/xip_payload/Makefile -- Task 4 (QSPI XIP cosim)
# minimal SH-2 payload build. See payload.S for the program itself (store
# the signature 0xF1A5B007 to boot-RAM byte address 0x00000100, then spin)
# and payload.ld for its link script (linked to run AT 0x14000000, the
# flash XIP entry Task 3's boot ROM vector table jumps to).
#
# Usage:
#   make            # (re)build payload.bin from payload.S
#   make clean
#
# payload.bin is committed (32 bytes, byte-for-byte reproducible from
# payload.S) so sim/xip_sim.sh does not need a cross-compiler at cosim
# time; `make` here is only needed if payload.S changes.
AS      := sh2-elf-as
LD      := sh2-elf-ld
OBJCOPY := sh2-elf-objcopy

all: payload.bin

payload.o: payload.S
	$(AS) -o $@ $<

payload.elf: payload.o payload.ld
	$(LD) -T payload.ld -o $@ payload.o

payload.bin: payload.elf
	$(OBJCOPY) -O binary $< $@

clean:
	rm -f payload.o payload.elf payload.bin

.PHONY: all clean
