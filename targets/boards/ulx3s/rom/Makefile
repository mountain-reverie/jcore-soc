CROSS_COMPILE ?= sh2-elf-
TOP_DIR       := $(realpath ../../../..)
BOARD_DIR     := $(realpath ..)

# The boot/ submodule hardcodes -mj2 which is not recognised by a plain
# sh2-elf GCC 14 toolchain. Use a compiler wrapper that translates -mj2 → -m2.
WRAPPER_DIR   := $(CURDIR)/.cc-wrapper
WRAPPER_CC    := $(WRAPPER_DIR)/sh2-elf-gcc

$(WRAPPER_DIR):
	mkdir -p $@

$(WRAPPER_CC): | $(WRAPPER_DIR)
	@printf '#!/bin/bash\nARGS=()\nfor a in "$$@"; do [[ "$$a" == "-mj2" ]] && ARGS+=("-m2") || ARGS+=("$$a"); done\nexec /usr/bin/env sh2-elf-gcc "$${ARGS[@]}"\n' > $@
	chmod +x $@

BOOT_FLAGS = \
		CROSS_COMPILE=$(CROSS_COMPILE) \
		CC=$(WRAPPER_CC) \
		EXTRA_CFLAGS="-I$(BOARD_DIR) -Wno-dangling-pointer" \
		CONFIG_DDR=0 \
		CONFIG_GDB_STUB=0 \
		CONFIG_GRLCD=0 \
		CONFIG_LCD=0 \
		CONFIG_LCD_LOGO=0 \
		CONFIG_SEVENSEGMENT=0 \
		CONFIG_SDCARD=1 \
		CONFIG_SPIFLASH=0 \
		CONFIG_LOAD_ELF=1 \
		CONFIG_DEVTREE_READ=1 \
		CONFIG_DEVTREE_FILENAME=dt.dtb \
		CONFIG_UARTLITE=1 \
		CONFIG_CPU_TESTS=0 \
		CONFIG_TEST_MEM=0 \
		CONFIG_SPI_LOOPBACK_TEST=1 \
		CONFIG_CPU1_DIAG=0 \
		NEED_LIBC=1

boot.elf: $(WRAPPER_CC)
	$(MAKE) -C $(TOP_DIR)/boot $(BOOT_FLAGS) clean
	mkdir -p $(TOP_DIR)/boot/lib
	$(MAKE) -C $(TOP_DIR)/boot $(BOOT_FLAGS) all
	cp $(TOP_DIR)/boot/bin/boot.elf $@

boot.bin: boot.elf
	$(CROSS_COMPILE)objcopy -O binary $< $@

all: boot.bin

clean:
	rm -f boot.elf boot.bin
	rm -rf $(WRAPPER_DIR)
	$(MAKE) -C $(TOP_DIR)/boot clean
